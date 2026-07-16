// Package wasmgc implements the experimental direct WebAssembly GC backend.
//
// Managed pointers are emitted as an unboxed WebAssembly GC reference and
// virtual byte offset. Keeping the reference in Wasm locals and struct fields
// lets the host collector trace it, including while JSPI suspends execution.
package wasmgc

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Program is the whole-program input to the WasmGC compiler.
type Program struct {
	Main *ssa.Package
}

type compiler struct {
	pkg         *ssa.Package
	functions   []*ssa.Function
	functionIDs map[*ssa.Function]int
	goIDs       map[*ssa.Go]int
	structs     []*structType
	structByGo  map[types.Type]*structType
}

type structType struct {
	id     int
	goType types.Type
	fields []field
}

type field struct {
	goType        types.Type
	virtualOffset int64
	physicalIndex int
	pointer       bool
	target        *structType
}

type valueInfo struct {
	goType types.Type
	base   *structType
	field  int
}

type functionCompiler struct {
	compiler *compiler
	fn       *ssa.Function
	values   map[ssa.Value]valueInfo
}

// Compile lowers the main package to a standalone WebAssembly GC module.
func Compile(program Program) (string, error) {
	if program.Main == nil {
		return "", fmt.Errorf("wasm-gc: missing main package")
	}
	program.Main.Build()

	c := &compiler{
		pkg:         program.Main,
		functionIDs: make(map[*ssa.Function]int),
		goIDs:       make(map[*ssa.Go]int),
		structByGo:  make(map[types.Type]*structType),
	}
	if err := c.findFunctions(); err != nil {
		return "", err
	}
	if c.hasNontrivialInit() {
		return "", c.errorAt(c.pkg.Func("init").Pos(), "package initialization is not supported")
	}
	if err := c.findStructs(); err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString("(module\n")
	if len(c.structs) != 0 {
		out.WriteString("  (rec\n")
		for _, typ := range c.structs {
			fmt.Fprintf(&out, "    (type $type%d (struct", typ.id)
			for _, field := range typ.fields {
				if field.pointer {
					fmt.Fprintf(&out, "\n      (field (mut (ref null $type%d)))", field.target.id)
					out.WriteString("\n      (field (mut i32))")
				} else {
					fmt.Fprintf(&out, "\n      (field (mut %s))", wasmScalarType(field.goType))
				}
			}
			out.WriteString("))\n")
		}
		out.WriteString("  )\n")
	}
	out.WriteString("  (import \"env\" \"printInt\" (func $printInt (param i32)))\n")
	out.WriteString("  (import \"env\" \"suspend\" (func $suspend (result i32)))\n")
	out.WriteString("  (import \"env\" \"makeChan\" (func $makeChan (param i32) (result i32)))\n")
	out.WriteString("  (import \"env\" \"channelSend\" (func $channelSend (param i32 i32) (result i32)))\n")
	out.WriteString("  (import \"env\" \"channelRecv\" (func $channelRecv (param i32) (result i32)))\n")
	goCalls := make([]*ssa.Go, len(c.goIDs))
	for instruction, id := range c.goIDs {
		goCalls[id] = instruction
	}
	for id, instruction := range goCalls {
		fmt.Fprintf(&out, "  (import \"env\" \"spawn%d\" (func $spawn%d", id, id)
		for _, arg := range instruction.Call.Args {
			if wasmScalarType(arg.Type()) != "i32" {
				return "", c.errorAt(instruction.Pos(), "goroutine arguments must be 32-bit scalar values")
			}
			out.WriteString(" (param i32)")
		}
		out.WriteString("))\n")
	}

	for _, fn := range c.functions {
		fc := &functionCompiler{
			compiler: c,
			fn:       fn,
			values:   make(map[ssa.Value]valueInfo),
		}
		text, err := fc.compile()
		if err != nil {
			return "", err
		}
		out.WriteString(text)
	}
	for id, instruction := range goCalls {
		callee := instruction.Call.StaticCallee()
		fmt.Fprintf(&out, "  (func (export \"goroutine%d\")", id)
		for i := range instruction.Call.Args {
			fmt.Fprintf(&out, " (param $arg%d i32)", i)
		}
		fmt.Fprintf(&out, "\n    (call $fn%d", c.functionIDs[callee])
		for i := range instruction.Call.Args {
			fmt.Fprintf(&out, " (local.get $arg%d)", i)
		}
		out.WriteString("))\n")
	}

	mainFn := c.pkg.Func("main")
	out.WriteString("  (func (export \"run\") (result i32)\n")
	fmt.Fprintf(&out, "    (call $fn%d)\n", c.functionIDs[mainFn])
	out.WriteString("    (i32.const 0))\n")
	out.WriteString(")\n")
	return out.String(), nil
}

func (c *compiler) hasNontrivialInit() bool {
	initFn := c.pkg.Func("init")
	if initFn == nil {
		return false
	}
	for _, block := range initFn.Blocks {
		for _, instruction := range block.Instrs {
			switch instruction := instruction.(type) {
			case *ssa.UnOp:
				global, ok := instruction.X.(*ssa.Global)
				if !ok || global.Name() != "init$guard" {
					return true
				}
			case *ssa.Store:
				global, ok := instruction.Addr.(*ssa.Global)
				if !ok || global.Name() != "init$guard" {
					return true
				}
			case *ssa.If, *ssa.Jump, *ssa.Return, *ssa.DebugRef:
			default:
				return true
			}
		}
	}
	return false
}

func (c *compiler) findFunctions() error {
	mainFn := c.pkg.Func("main")
	if mainFn == nil {
		return fmt.Errorf("wasm-gc: main.main not found")
	}

	var visit func(*ssa.Function) error
	visit = func(fn *ssa.Function) error {
		if _, ok := c.functionIDs[fn]; ok {
			return nil
		}
		if fn.Pkg != c.pkg {
			return c.errorAt(fn.Pos(), "calls outside the main package are not supported: %s", fn.String())
		}
		c.functionIDs[fn] = len(c.functions)
		c.functions = append(c.functions, fn)
		for _, block := range fn.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if ok {
					callee := call.Call.StaticCallee()
					if callee != nil {
						if err := visit(callee); err != nil {
							return err
						}
					}
				}
				goInstruction, ok := instruction.(*ssa.Go)
				if ok {
					callee := goInstruction.Call.StaticCallee()
					if callee == nil {
						return c.errorAt(goInstruction.Pos(), "indirect goroutine calls are not supported")
					}
					if _, exists := c.goIDs[goInstruction]; !exists {
						c.goIDs[goInstruction] = len(c.goIDs)
					}
					if err := visit(callee); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}

	return visit(mainFn)
}

func (c *compiler) findStructs() error {
	var addType func(types.Type) error
	addType = func(goType types.Type) error {
		goType = dereference(goType)
		if _, ok := c.structByGo[goType]; ok {
			return nil
		}
		underlying, ok := goType.Underlying().(*types.Struct)
		if !ok {
			return fmt.Errorf("wasm-gc: allocation type is not a struct: %s", goType)
		}
		typ := &structType{id: len(c.structs), goType: goType}
		c.structByGo[goType] = typ
		c.structs = append(c.structs, typ)

		sizes := types.SizesFor("gc", "wasm")
		vars := make([]*types.Var, underlying.NumFields())
		for i := range vars {
			vars[i] = underlying.Field(i)
		}
		offsets := sizes.Offsetsof(vars)
		physicalIndex := 0
		for i := 0; i < underlying.NumFields(); i++ {
			fieldType := underlying.Field(i).Type()
			f := field{
				goType:        fieldType,
				virtualOffset: offsets[i],
				physicalIndex: physicalIndex,
			}
			if pointer, ok := fieldType.Underlying().(*types.Pointer); ok {
				targetType := dereference(pointer)
				if _, ok := targetType.Underlying().(*types.Struct); !ok {
					return fmt.Errorf("wasm-gc: pointer field %s.%s does not point to a struct", goType, underlying.Field(i).Name())
				}
				if err := addType(targetType); err != nil {
					return err
				}
				f.pointer = true
				f.target = c.structByGo[targetType]
				physicalIndex += 2
			} else {
				if !isScalar(fieldType) {
					return fmt.Errorf("wasm-gc: unsupported field type %s.%s: %s", goType, underlying.Field(i).Name(), fieldType)
				}
				physicalIndex++
			}
			typ.fields = append(typ.fields, f)
		}
		return nil
	}

	for _, fn := range c.functions {
		for _, block := range fn.Blocks {
			for _, instruction := range block.Instrs {
				if alloc, ok := instruction.(*ssa.Alloc); ok {
					if err := addType(alloc.Type()); err != nil {
						return c.errorAt(alloc.Pos(), "%v", err)
					}
				}
			}
		}
	}
	return nil
}

func (fc *functionCompiler) compile() (string, error) {
	if len(fc.fn.Blocks) == 0 {
		return "", fc.compiler.errorAt(fc.fn.Pos(), "function %s has no body", fc.fn.String())
	}

	for _, param := range fc.fn.Params {
		info, err := fc.infoForType(param.Type())
		if err != nil {
			return "", fc.compiler.errorAt(param.Pos(), "%v", err)
		}
		fc.values[param] = info
	}
	for _, block := range fc.fn.Blocks {
		for _, instruction := range block.Instrs {
			value, ok := instruction.(ssa.Value)
			if !ok {
				continue
			}
			info, err := fc.analyzeValue(value)
			if err != nil {
				return "", err
			}
			fc.values[value] = info
		}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "  (func $fn%d", fc.compiler.functionIDs[fc.fn])
	for _, param := range fc.fn.Params {
		writeDeclaration(&out, "param", valueName(param), fc.values[param])
	}
	for i := 0; i < fc.fn.Signature.Results().Len(); i++ {
		info, err := fc.infoForType(fc.fn.Signature.Results().At(i).Type())
		if err != nil {
			return "", fc.compiler.errorAt(fc.fn.Pos(), "%v", err)
		}
		if info.base != nil {
			fmt.Fprintf(&out, " (result (ref null $type%d)) (result i32)", info.base.id)
		} else {
			fmt.Fprintf(&out, " (result %s)", wasmScalarType(info.goType))
		}
	}
	out.WriteString("\n")

	for _, block := range fc.fn.Blocks {
		for _, instruction := range block.Instrs {
			value, ok := instruction.(ssa.Value)
			if !ok || isZeroTuple(value.Type()) {
				continue
			}
			writeDeclaration(&out, "local", valueName(value), fc.values[value])
			if phi, ok := value.(*ssa.Phi); ok {
				writeDeclaration(&out, "local", phiTempName(phi), fc.values[value])
			}
		}
	}

	if len(fc.fn.Blocks) == 1 {
		for _, instruction := range fc.fn.Blocks[0].Instrs {
			if _, ok := instruction.(*ssa.Phi); ok {
				continue
			}
			if err := fc.emitInstruction(&out, instruction); err != nil {
				return "", err
			}
		}
	} else {
		out.WriteString("    (local $block i32)\n")
		out.WriteString("    (loop $dispatch\n")
		for _, block := range fc.fn.Blocks {
			fmt.Fprintf(&out, "      (if (i32.eq (local.get $block) (i32.const %d))\n", block.Index)
			out.WriteString("        (then\n")
			for _, instruction := range block.Instrs {
				switch instruction := instruction.(type) {
				case *ssa.Phi:
					continue
				case *ssa.Jump:
					if err := fc.emitEdge(&out, block, block.Succs[0]); err != nil {
						return "", err
					}
				case *ssa.If:
					fmt.Fprintf(&out, "          (if %s\n", fc.expression(instruction.Cond))
					out.WriteString("            (then\n")
					if err := fc.emitEdge(&out, block, block.Succs[0]); err != nil {
						return "", err
					}
					out.WriteString("            )\n")
					out.WriteString("            (else\n")
					if err := fc.emitEdge(&out, block, block.Succs[1]); err != nil {
						return "", err
					}
					out.WriteString("            ))\n")
				default:
					if err := fc.emitInstruction(&out, instruction); err != nil {
						return "", err
					}
				}
			}
			out.WriteString("        ))\n")
		}
		out.WriteString("      (unreachable))\n")
	}
	out.WriteString("  )\n")
	return out.String(), nil
}

func (fc *functionCompiler) emitEdge(out *strings.Builder, predecessor, successor *ssa.BasicBlock) error {
	predecessorIndex := -1
	for i, block := range successor.Preds {
		if block == predecessor {
			predecessorIndex = i
			break
		}
	}
	if predecessorIndex < 0 {
		return fc.compiler.errorAt(fc.fn.Pos(), "invalid SSA edge from block %d to block %d", predecessor.Index, successor.Index)
	}

	var phis []*ssa.Phi
	for _, instruction := range successor.Instrs {
		phi, ok := instruction.(*ssa.Phi)
		if !ok {
			break
		}
		if predecessorIndex >= len(phi.Edges) {
			return fc.compiler.errorAt(phi.Pos(), "missing phi edge from block %d", predecessor.Index)
		}
		phis = append(phis, phi)
		info := fc.values[phi]
		edge := phi.Edges[predecessorIndex]
		if info.base != nil {
			base, offset := fc.pointerExpressions(edge, info)
			fmt.Fprintf(out, "              (local.set %s_base %s)\n", phiTempName(phi), base)
			fmt.Fprintf(out, "              (local.set %s_offset %s)\n", phiTempName(phi), offset)
		} else {
			fmt.Fprintf(out, "              (local.set %s %s)\n", phiTempName(phi), fc.expression(edge))
		}
	}
	for _, phi := range phis {
		info := fc.values[phi]
		if info.base != nil {
			fmt.Fprintf(out, "              (local.set %s (local.get %s_base))\n", baseName(phi), phiTempName(phi))
			fmt.Fprintf(out, "              (local.set %s (local.get %s_offset))\n", offsetName(phi), phiTempName(phi))
		} else {
			fmt.Fprintf(out, "              (local.set %s (local.get %s))\n", valueName(phi), phiTempName(phi))
		}
	}
	fmt.Fprintf(out, "              (local.set $block (i32.const %d))\n", successor.Index)
	out.WriteString("              (br $dispatch)\n")
	return nil
}

func (fc *functionCompiler) analyzeValue(value ssa.Value) (valueInfo, error) {
	switch value := value.(type) {
	case *ssa.Phi:
		return fc.infoForType(value.Type())
	case *ssa.Alloc:
		base := fc.compiler.structByGo[dereference(value.Type())]
		if base == nil {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "unsupported allocation type: %s", value.Type())
		}
		return valueInfo{goType: value.Type(), base: base, field: -1}, nil
	case *ssa.FieldAddr:
		source, err := fc.info(value.X)
		if err != nil {
			return valueInfo{}, err
		}
		base := source.base
		if base == nil {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "field address has no managed base")
		}
		if value.Field < 0 || value.Field >= len(base.fields) {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "invalid field index %d", value.Field)
		}
		return valueInfo{goType: value.Type(), base: base, field: value.Field}, nil
	case *ssa.UnOp:
		if value.Op == token.ARROW {
			if wasmScalarType(value.Type()) != "i32" {
				return valueInfo{}, fc.compiler.errorAt(value.Pos(), "channels only support 32-bit scalar elements")
			}
			return valueInfo{goType: value.Type(), field: -1}, nil
		}
		if value.Op != token.MUL {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "unsupported unary operation: %s", value.Op)
		}
		source, err := fc.info(value.X)
		if err != nil {
			return valueInfo{}, err
		}
		if source.field < 0 {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "loading whole structs is not supported")
		}
		field := source.base.fields[source.field]
		if field.pointer {
			return valueInfo{goType: value.Type(), base: field.target, field: -1}, nil
		}
		return valueInfo{goType: value.Type(), field: -1}, nil
	case *ssa.BinOp:
		if !isScalar(value.Type()) {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "unsupported binary result type: %s", value.Type())
		}
		return valueInfo{goType: value.Type(), field: -1}, nil
	case *ssa.Call:
		if isZeroTuple(value.Type()) {
			return valueInfo{goType: value.Type(), field: -1}, nil
		}
		return fc.infoForType(value.Type())
	case *ssa.MakeChan:
		if wasmScalarType(value.Type()) != "i32" {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "unsupported channel type: %s", value.Type())
		}
		return valueInfo{goType: value.Type(), field: -1}, nil
	case *ssa.ChangeType, *ssa.Convert:
		return fc.infoForType(value.Type())
	default:
		return valueInfo{}, fc.compiler.errorAt(value.Pos(), "unsupported SSA value: %T", value)
	}
}

func (fc *functionCompiler) emitInstruction(out *strings.Builder, instruction ssa.Instruction) error {
	switch instruction := instruction.(type) {
	case *ssa.Alloc:
		info := fc.values[instruction]
		fmt.Fprintf(out, "    (local.set %s (struct.new_default $type%d))\n", baseName(instruction), info.base.id)
		fmt.Fprintf(out, "    (local.set %s (i32.const 0))\n", offsetName(instruction))
		out.WriteString("    (drop (call $suspend))\n")
	case *ssa.FieldAddr:
		source := fc.values[instruction.X]
		field := source.base.fields[instruction.Field]
		fmt.Fprintf(out, "    (local.set %s (local.get %s))\n", baseName(instruction), baseName(instruction.X))
		fmt.Fprintf(out, "    (local.set %s (i32.add (local.get %s) (i32.const %d)))\n", offsetName(instruction), offsetName(instruction.X), field.virtualOffset)
	case *ssa.Store:
		return fc.emitStore(out, instruction)
	case *ssa.UnOp:
		if instruction.Op == token.ARROW {
			fmt.Fprintf(out, "    (local.set %s (call $channelRecv %s))\n", valueName(instruction), fc.expression(instruction.X))
			return nil
		}
		return fc.emitLoad(out, instruction)
	case *ssa.BinOp:
		return fc.emitBinOp(out, instruction)
	case *ssa.Call:
		return fc.emitCall(out, instruction)
	case *ssa.MakeChan:
		fmt.Fprintf(out, "    (local.set %s (call $makeChan %s))\n", valueName(instruction), fc.expression(instruction.Size))
	case *ssa.Send:
		if wasmScalarType(instruction.X.Type()) != "i32" {
			return fc.compiler.errorAt(instruction.Pos(), "channels only support 32-bit scalar elements")
		}
		fmt.Fprintf(out, "    (drop (call $channelSend %s %s))\n", fc.expression(instruction.Chan), fc.expression(instruction.X))
	case *ssa.Go:
		id, ok := fc.compiler.goIDs[instruction]
		if !ok {
			return fc.compiler.errorAt(instruction.Pos(), "missing goroutine identifier")
		}
		fmt.Fprintf(out, "    (call $spawn%d", id)
		for _, arg := range instruction.Call.Args {
			if wasmScalarType(arg.Type()) != "i32" {
				return fc.compiler.errorAt(instruction.Pos(), "goroutine arguments must be 32-bit scalar values")
			}
			fmt.Fprintf(out, " %s", fc.expression(arg))
		}
		out.WriteString(")\n")
	case *ssa.ChangeType:
		fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), fc.expression(instruction.X))
	case *ssa.Convert:
		fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), fc.expression(instruction.X))
	case *ssa.Return:
		out.WriteString("    (return")
		for _, result := range instruction.Results {
			info, err := fc.info(result)
			if err != nil {
				return err
			}
			if info.base != nil {
				base, offset := fc.pointerExpressions(result, info)
				fmt.Fprintf(out, " %s %s", base, offset)
			} else {
				fmt.Fprintf(out, " %s", fc.expression(result))
			}
		}
		out.WriteString(")\n")
	case *ssa.DebugRef:
	default:
		return fc.compiler.errorAt(instruction.Pos(), "unsupported SSA instruction: %T", instruction)
	}
	return nil
}

func (fc *functionCompiler) emitBinOp(out *strings.Builder, instruction *ssa.BinOp) error {
	left, err := fc.info(instruction.X)
	if err != nil {
		return err
	}
	right, err := fc.info(instruction.Y)
	if err != nil {
		return err
	}
	if left.base != nil || right.base != nil {
		if left.base == nil || right.base == nil || left.base != right.base {
			return fc.compiler.errorAt(instruction.Pos(), "pointer comparison has incompatible managed bases")
		}
		if instruction.Op != token.EQL && instruction.Op != token.NEQ {
			return fc.compiler.errorAt(instruction.Pos(), "unsupported pointer comparison: %s", instruction.Op)
		}
		leftBase, leftOffset := fc.pointerExpressions(instruction.X, left)
		rightBase, rightOffset := fc.pointerExpressions(instruction.Y, right)
		expression := fmt.Sprintf("(i32.and (ref.eq %s %s) (i32.eq %s %s))", leftBase, rightBase, leftOffset, rightOffset)
		if instruction.Op == token.NEQ {
			expression = "(i32.eqz " + expression + ")"
		}
		fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), expression)
		return nil
	}

	op, err := wasmBinOp(instruction.Op, instruction.X.Type())
	if err != nil {
		return fc.compiler.errorAt(instruction.Pos(), "%v", err)
	}
	fmt.Fprintf(out, "    (local.set %s (%s %s %s))\n", valueName(instruction), op, fc.expression(instruction.X), fc.expression(instruction.Y))
	return nil
}

func (fc *functionCompiler) emitStore(out *strings.Builder, instruction *ssa.Store) error {
	address, err := fc.info(instruction.Addr)
	if err != nil {
		return err
	}
	if address.base == nil || address.field < 0 {
		return fc.compiler.errorAt(instruction.Pos(), "store destination is not a managed struct field")
	}
	field := address.base.fields[address.field]
	receiver := fmt.Sprintf("(ref.as_non_null (local.get %s))", baseName(instruction.Addr))
	if field.pointer {
		value, err := fc.info(instruction.Val)
		if err != nil {
			return err
		}
		if value.base != field.target {
			return fc.compiler.errorAt(instruction.Pos(), "pointer store has incompatible managed base")
		}
		base, offset := fc.pointerExpressions(instruction.Val, value)
		fmt.Fprintf(out, "    (struct.set $type%d %d %s %s)\n", address.base.id, field.physicalIndex, receiver, base)
		fmt.Fprintf(out, "    (struct.set $type%d %d %s %s)\n", address.base.id, field.physicalIndex+1, receiver, offset)
	} else {
		fmt.Fprintf(out, "    (struct.set $type%d %d %s %s)\n", address.base.id, field.physicalIndex, receiver, fc.expression(instruction.Val))
	}
	return nil
}

func (fc *functionCompiler) emitLoad(out *strings.Builder, instruction *ssa.UnOp) error {
	source, err := fc.info(instruction.X)
	if err != nil {
		return err
	}
	if source.base == nil || source.field < 0 {
		return fc.compiler.errorAt(instruction.Pos(), "load source is not a managed struct field")
	}
	field := source.base.fields[source.field]
	receiver := fmt.Sprintf("(ref.as_non_null (local.get %s))", baseName(instruction.X))
	if field.pointer {
		fmt.Fprintf(out, "    (local.set %s (struct.get $type%d %d %s))\n", baseName(instruction), source.base.id, field.physicalIndex, receiver)
		fmt.Fprintf(out, "    (local.set %s (struct.get $type%d %d %s))\n", offsetName(instruction), source.base.id, field.physicalIndex+1, receiver)
	} else {
		fmt.Fprintf(out, "    (local.set %s (struct.get $type%d %d %s))\n", valueName(instruction), source.base.id, field.physicalIndex, receiver)
	}
	return nil
}

func (fc *functionCompiler) emitCall(out *strings.Builder, instruction *ssa.Call) error {
	if builtin, ok := instruction.Call.Value.(*ssa.Builtin); ok {
		switch builtin.Name() {
		case "println", "print":
			for _, arg := range instruction.Call.Args {
				info, err := fc.info(arg)
				if err != nil {
					return err
				}
				if info.base != nil || wasmScalarType(info.goType) != "i32" {
					return fc.compiler.errorAt(instruction.Pos(), "%s only supports 32-bit scalar values", builtin.Name())
				}
				fmt.Fprintf(out, "    (call $printInt %s)\n", fc.expression(arg))
			}
			return nil
		default:
			return fc.compiler.errorAt(instruction.Pos(), "unsupported builtin: %s", builtin.Name())
		}
	}

	callee := instruction.Call.StaticCallee()
	if callee == nil {
		return fc.compiler.errorAt(instruction.Pos(), "indirect calls are not supported")
	}
	var call strings.Builder
	fmt.Fprintf(&call, "(call $fn%d", fc.compiler.functionIDs[callee])
	for _, arg := range instruction.Call.Args {
		info, err := fc.info(arg)
		if err != nil {
			return err
		}
		if info.base != nil {
			base, offset := fc.pointerExpressions(arg, info)
			fmt.Fprintf(&call, " %s %s", base, offset)
		} else {
			fmt.Fprintf(&call, " %s", fc.expression(arg))
		}
	}
	call.WriteString(")")

	if isZeroTuple(instruction.Type()) {
		fmt.Fprintf(out, "    %s\n", call.String())
		return nil
	}
	info := fc.values[instruction]
	if info.base != nil {
		fmt.Fprintf(out, "    %s\n", call.String())
		fmt.Fprintf(out, "    (local.set %s)\n", offsetName(instruction))
		fmt.Fprintf(out, "    (local.set %s)\n", baseName(instruction))
	} else {
		fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), call.String())
	}
	return nil
}

func (fc *functionCompiler) expression(value ssa.Value) string {
	if value, ok := value.(*ssa.Const); ok {
		switch {
		case value.IsNil():
			return "(i32.const 0)"
		case value.Value.Kind() == constant.Bool:
			if constant.BoolVal(value.Value) {
				return "(i32.const 1)"
			}
			return "(i32.const 0)"
		case value.Value.Kind() == constant.Int:
			number, exact := constant.Int64Val(value.Value)
			if !exact {
				panic("integer constant does not fit in int64")
			}
			return "(i32.const " + strconv.FormatInt(number, 10) + ")"
		default:
			panic("unsupported constant: " + value.String())
		}
	}
	return "(local.get " + valueName(value) + ")"
}

func (fc *functionCompiler) pointerExpressions(value ssa.Value, info valueInfo) (string, string) {
	if constant, ok := value.(*ssa.Const); ok && constant.IsNil() {
		return fmt.Sprintf("(ref.null $type%d)", info.base.id), "(i32.const 0)"
	}
	return "(local.get " + baseName(value) + ")", "(local.get " + offsetName(value) + ")"
}

func (fc *functionCompiler) info(value ssa.Value) (valueInfo, error) {
	if constant, ok := value.(*ssa.Const); ok {
		return fc.infoForType(constant.Type())
	}
	info, ok := fc.values[value]
	if !ok {
		return valueInfo{}, fc.compiler.errorAt(value.Pos(), "missing value information for %s", value.Name())
	}
	return info, nil
}

func (fc *functionCompiler) infoForType(goType types.Type) (valueInfo, error) {
	if pointer, ok := goType.Underlying().(*types.Pointer); ok {
		target := dereference(pointer)
		if _, ok := target.Underlying().(*types.Struct); !ok {
			return valueInfo{}, fmt.Errorf("wasm-gc: pointer type has no statically known managed base: %s", goType)
		}
		base := fc.compiler.structByGo[target]
		if base == nil {
			return valueInfo{}, fmt.Errorf("wasm-gc: managed type was not discovered: %s", target)
		}
		return valueInfo{goType: goType, base: base, field: -1}, nil
	}
	if !isScalar(goType) && !isZeroTuple(goType) {
		if _, ok := goType.Underlying().(*types.Chan); ok {
			return valueInfo{goType: goType, field: -1}, nil
		}
		return valueInfo{}, fmt.Errorf("wasm-gc: unsupported value type: %s", goType)
	}
	return valueInfo{goType: goType, field: -1}, nil
}

func (c *compiler) errorAt(pos token.Pos, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if pos == token.NoPos {
		return fmt.Errorf("wasm-gc: %s", message)
	}
	return fmt.Errorf("%s: wasm-gc: %s", c.pkg.Prog.Fset.Position(pos), message)
}

func dereference(goType types.Type) types.Type {
	if pointer, ok := goType.Underlying().(*types.Pointer); ok {
		return pointer.Elem()
	}
	return goType
}

func isScalar(goType types.Type) bool {
	basic, ok := goType.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch basic.Kind() {
	case types.Bool,
		types.Int, types.Int32,
		types.Uint, types.Uint32, types.Uintptr:
		return true
	default:
		return false
	}
}

func wasmScalarType(goType types.Type) string {
	if _, ok := goType.Underlying().(*types.Chan); ok {
		return "i32"
	}
	if isScalar(goType) {
		return "i32"
	}
	return ""
}

func wasmBinOp(op token.Token, goType types.Type) (string, error) {
	prefix := wasmScalarType(goType)
	comparisonSuffix := "_s"
	if basic, ok := goType.Underlying().(*types.Basic); ok {
		switch basic.Kind() {
		case types.Uint, types.Uint32, types.Uintptr:
			comparisonSuffix = "_u"
		}
	}
	switch op {
	case token.EQL:
		return prefix + ".eq", nil
	case token.NEQ:
		return prefix + ".ne", nil
	case token.LSS:
		return prefix + ".lt" + comparisonSuffix, nil
	case token.LEQ:
		return prefix + ".le" + comparisonSuffix, nil
	case token.GTR:
		return prefix + ".gt" + comparisonSuffix, nil
	case token.GEQ:
		return prefix + ".ge" + comparisonSuffix, nil
	case token.ADD:
		return prefix + ".add", nil
	case token.SUB:
		return prefix + ".sub", nil
	case token.MUL:
		return prefix + ".mul", nil
	case token.AND:
		return prefix + ".and", nil
	case token.OR:
		return prefix + ".or", nil
	case token.XOR:
		return prefix + ".xor", nil
	default:
		return "", fmt.Errorf("unsupported binary operation: %s", op)
	}
}

func valueName(value ssa.Value) string {
	return "$" + value.Name()
}

func baseName(value ssa.Value) string {
	return valueName(value) + "_base"
}

func offsetName(value ssa.Value) string {
	return valueName(value) + "_offset"
}

func phiTempName(phi *ssa.Phi) string {
	return valueName(phi) + "_phi"
}

func writeDeclaration(out *strings.Builder, kind, name string, info valueInfo) {
	if info.base != nil {
		fmt.Fprintf(out, " (%s %s_base (ref null $type%d))", kind, name, info.base.id)
		fmt.Fprintf(out, " (%s %s_offset i32)", kind, name)
	} else {
		fmt.Fprintf(out, " (%s %s %s)", kind, name, wasmScalarType(info.goType))
	}
}

func isZeroTuple(goType types.Type) bool {
	tuple, ok := goType.(*types.Tuple)
	return ok && tuple.Len() == 0
}
