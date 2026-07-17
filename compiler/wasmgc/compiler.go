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
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Program is the whole-program input to the WasmGC compiler.
type Program struct {
	Main *ssa.Package
}

type compiler struct {
	pkg             *ssa.Package
	functions       []*ssa.Function
	functionIDs     map[*ssa.Function]int
	initFunctions   []*ssa.Function
	goIDs           map[*ssa.Go]int
	structs         []*structType
	structByGo      map[types.Type]*structType
	boxByGo         map[types.Type]*structType
	arrays          []*arrayType
	arrayByElem     map[types.Type]*arrayType
	stringArray     *arrayType
	strings         []string
	stringIDs       map[string]int
	globals         []*globalInfo
	globalBySSA     map[*ssa.Global]*globalInfo
	closureBindings map[*ssa.Function][]ssa.Value
	knownValues     map[ssa.Value]valueInfo
}

type structType struct {
	id     int
	goType types.Type
	fields []field
}

type arrayType struct {
	id            int
	element       types.Type
	elementSize   int64
	elementBox    *structType
	elementStruct *structType
}

type field struct {
	goType        types.Type
	virtualOffset int64
	physicalIndex int
	pointer       bool
	target        *structType
}

type valueInfo struct {
	goType       types.Type
	base         *structType
	array        *arrayType
	elementArray *arrayType
	structValue  bool
	slice        bool
	string       bool
	length       int64
	field        int
	global       *globalInfo
	closure      *ssa.MakeClosure
}

type globalInfo struct {
	id          int
	valueType   types.Type
	base        *structType
	array       *arrayType
	structValue bool
	slice       bool
	string      bool
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
		pkg:             program.Main,
		functionIDs:     make(map[*ssa.Function]int),
		goIDs:           make(map[*ssa.Go]int),
		structByGo:      make(map[types.Type]*structType),
		boxByGo:         make(map[types.Type]*structType),
		arrayByElem:     make(map[types.Type]*arrayType),
		stringIDs:       make(map[string]int),
		globalBySSA:     make(map[*ssa.Global]*globalInfo),
		closureBindings: make(map[*ssa.Function][]ssa.Value),
		knownValues:     make(map[ssa.Value]valueInfo),
	}
	if err := c.findFunctions(); err != nil {
		return "", err
	}
	if err := c.findArrays(); err != nil {
		return "", err
	}
	if err := c.findStructs(); err != nil {
		return "", err
	}
	if err := c.findGlobals(); err != nil {
		return "", err
	}

	var functions strings.Builder
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
		functions.WriteString(text)
	}

	var out strings.Builder
	out.WriteString("(module\n")
	if len(c.arrays) != 0 || len(c.structs) != 0 {
		out.WriteString("  (rec\n")
		for _, typ := range c.arrays {
			fmt.Fprintf(&out, "    (type $array%d (array (mut %s)))\n", typ.id, wasmArrayElementType(typ))
		}
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
	goCalls := make([]*ssa.Go, len(c.goIDs))
	for instruction, id := range c.goIDs {
		goCalls[id] = instruction
	}
	if len(goCalls) != 0 {
		out.WriteString("  (rec\n")
		for id, instruction := range goCalls {
			fmt.Fprintf(&out, "    (type $task%d (struct\n", id)
			fmt.Fprintf(&out, "      (field (mut (ref null $task%d)))", id)
			for _, value := range goCallValues(instruction) {
				info, err := c.info(value)
				if err != nil {
					return "", err
				}
				for _, typ := range wasmValueTypes(info) {
					fmt.Fprintf(&out, "\n      (field %s)", typ)
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
	if len(goCalls) != 0 {
		out.WriteString("  (import \"env\" \"scheduleTask\" (func $scheduleTask (param i32)))\n")
	}
	for _, global := range c.globals {
		if global.structValue {
			fmt.Fprintf(&out, "  (global $global%d (mut (ref null $type%d)) (struct.new_default $type%d))\n", global.id, global.base.id, global.base.id)
		} else if global.array != nil {
			fmt.Fprintf(&out, "  (global $global%d_base (mut (ref null $array%d)) (ref.null $array%d))\n", global.id, global.array.id, global.array.id)
			fmt.Fprintf(&out, "  (global $global%d_offset (mut i32) (i32.const 0))\n", global.id)
			fmt.Fprintf(&out, "  (global $global%d_len (mut i32) (i32.const 0))\n", global.id)
			if global.slice {
				fmt.Fprintf(&out, "  (global $global%d_cap (mut i32) (i32.const 0))\n", global.id)
			}
		} else if global.base != nil {
			fmt.Fprintf(&out, "  (global $global%d_base (mut (ref null $type%d)) (ref.null $type%d))\n", global.id, global.base.id, global.base.id)
			fmt.Fprintf(&out, "  (global $global%d_offset (mut i32) (i32.const 0))\n", global.id)
		} else {
			fmt.Fprintf(&out, "  (global $global%d (mut i32) (i32.const 0))\n", global.id)
		}
	}
	for id := range c.strings {
		array := c.stringArray
		fmt.Fprintf(&out, "  (global $string%d (mut (ref null $array%d)) (ref.null $array%d))\n", id, array.id, array.id)
	}
	for id := range goCalls {
		fmt.Fprintf(&out, "  (global $tasks%d_head (mut (ref null $task%d)) (ref.null $task%d))\n", id, id, id)
		fmt.Fprintf(&out, "  (global $tasks%d_tail (mut (ref null $task%d)) (ref.null $task%d))\n", id, id, id)
	}
	if len(c.strings) != 0 {
		array := c.stringArray
		out.WriteString("  (func $initStrings\n")
		for id, value := range c.strings {
			fmt.Fprintf(&out, "    (global.set $string%d (array.new_fixed $array%d %d", id, array.id, len(value))
			for i := 0; i < len(value); i++ {
				fmt.Fprintf(&out, " (i32.const %d)", value[i])
			}
			out.WriteString("))\n")
		}
		out.WriteString("  )\n")
	}
	if c.stringArray != nil {
		c.writeStringEqual(&out, c.stringArray)
	}

	out.WriteString(functions.String())
	if len(goCalls) != 0 {
		if err := c.writeTaskRunner(&out, goCalls); err != nil {
			return "", err
		}
	}

	mainFn := c.pkg.Func("main")
	out.WriteString("  (func (export \"run\") (result i32)\n")
	if len(c.strings) != 0 {
		out.WriteString("    (call $initStrings)\n")
		out.WriteString("    (drop (call $suspend))\n")
	}
	for _, initFn := range c.initFunctions {
		fmt.Fprintf(&out, "    (call $fn%d)\n", c.functionIDs[initFn])
	}
	fmt.Fprintf(&out, "    (call $fn%d)\n", c.functionIDs[mainFn])
	out.WriteString("    (i32.const 0))\n")
	out.WriteString(")\n")
	return out.String(), nil
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
		if fn.Pkg == nil {
			return c.errorAt(fn.Pos(), "function has no Go package body: %s", fn.String())
		}
		fn.Pkg.Build()
		c.functionIDs[fn] = len(c.functions)
		c.functions = append(c.functions, fn)
		for _, block := range fn.Blocks {
			for _, instruction := range block.Instrs {
				if closure, ok := instruction.(*ssa.MakeClosure); ok {
					callee := closure.Fn.(*ssa.Function)
					if _, exists := c.closureBindings[callee]; !exists {
						c.closureBindings[callee] = closure.Bindings
					}
					if err := visit(callee); err != nil {
						return err
					}
				}
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
					callee := goCallee(goInstruction)
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

	initializedPackages := make(map[*ssa.Package]struct{})
	var visitPackage func(*ssa.Package) error
	visitPackage = func(pkg *ssa.Package) error {
		if _, ok := initializedPackages[pkg]; ok {
			return nil
		}
		initializedPackages[pkg] = struct{}{}
		for _, imported := range pkg.Pkg.Imports() {
			importedPkg := pkg.Prog.ImportedPackage(imported.Path())
			if importedPkg == nil {
				return c.errorAt(token.NoPos, "SSA package is unavailable for import %s", imported.Path())
			}
			if err := visitPackage(importedPkg); err != nil {
				return err
			}
		}
		pkg.Build()
		if initFn := pkg.Func("init"); initFn != nil {
			if err := visit(initFn); err != nil {
				return err
			}
			c.initFunctions = append(c.initFunctions, initFn)
		}
		return nil
	}
	if err := visitPackage(c.pkg); err != nil {
		return err
	}
	if err := visit(mainFn); err != nil {
		return err
	}
	return nil
}

func goCallee(instruction *ssa.Go) *ssa.Function {
	if closure, ok := instruction.Call.Value.(*ssa.MakeClosure); ok {
		return closure.Fn.(*ssa.Function)
	}
	return instruction.Call.StaticCallee()
}

func goCallValues(instruction *ssa.Go) []ssa.Value {
	var values []ssa.Value
	if closure, ok := instruction.Call.Value.(*ssa.MakeClosure); ok {
		values = append(values, closure.Bindings...)
	}
	return append(values, instruction.Call.Args...)
}

func (c *compiler) writeTaskRunner(out *strings.Builder, goCalls []*ssa.Go) error {
	out.WriteString("  (func (export \"runTask\") (param $site i32)\n")
	for id := range goCalls {
		fmt.Fprintf(out, "    (local $task%d (ref null $task%d))\n", id, id)
	}
	for id, instruction := range goCalls {
		fmt.Fprintf(out, "    (if (i32.eq (local.get $site) (i32.const %d))\n", id)
		out.WriteString("      (then\n")
		fmt.Fprintf(out, "        (if (ref.is_null (global.get $tasks%d_head)) (then (return)))\n", id)
		fmt.Fprintf(out, "        (local.set $task%d (global.get $tasks%d_head))\n", id, id)
		task := fmt.Sprintf("(ref.as_non_null (local.get $task%d))", id)
		fmt.Fprintf(out, "        (global.set $tasks%d_head (struct.get $task%d 0 %s))\n", id, id, task)
		fmt.Fprintf(out, "        (struct.set $task%d 0 %s (ref.null $task%d))\n", id, task, id)
		fmt.Fprintf(out, "        (if (ref.is_null (global.get $tasks%d_head))\n", id)
		fmt.Fprintf(out, "          (then (global.set $tasks%d_tail (ref.null $task%d))))\n", id, id)
		callee := goCallee(instruction)
		fmt.Fprintf(out, "        (call $fn%d", c.functionIDs[callee])
		fieldIndex := 1
		for _, value := range goCallValues(instruction) {
			info, err := c.info(value)
			if err != nil {
				return err
			}
			for range wasmValueTypes(info) {
				fmt.Fprintf(out, " (struct.get $task%d %d %s)", id, fieldIndex, task)
				fieldIndex++
			}
		}
		out.WriteString(")\n")
		for i := 0; i < callee.Signature.Results().Len(); i++ {
			info, err := c.infoForType(callee.Signature.Results().At(i).Type())
			if err != nil {
				return c.errorAt(instruction.Pos(), "%v", err)
			}
			for range wasmValueTypes(info) {
				out.WriteString("        (drop)\n")
			}
		}
		out.WriteString("        (return)))\n")
	}
	out.WriteString("  )\n")
	return nil
}

func (c *compiler) findArrays() error {
	addArray := func(element types.Type, pos token.Pos) error {
		if !isScalar(element) {
			if pointer, ok := element.Underlying().(*types.Pointer); ok {
				if _, ok := pointer.Elem().Underlying().(*types.Struct); !ok {
					return c.errorAt(pos, "managed pointer array element does not point to a struct: %s", element)
				}
			} else if _, ok := element.Underlying().(*types.Struct); !ok {
				return c.errorAt(pos, "unsupported managed array element type: %s", element)
			}
		}
		elementSize := types.SizesFor("gc", "wasm").Sizeof(element)
		if elementSize == 0 {
			if _, ok := element.Underlying().(*types.Struct); ok {
				elementSize = 1
			}
		}
		if elementSize <= 0 {
			return c.errorAt(pos, "unsupported managed array element type: %s", element)
		}
		if _, ok := c.arrayByElem[element]; ok {
			return nil
		}
		for existingElement, array := range c.arrayByElem {
			if types.Identical(element, existingElement) {
				c.arrayByElem[element] = array
				return nil
			}
		}
		typ := &arrayType{
			id:          len(c.arrays),
			element:     element,
			elementSize: elementSize,
		}
		c.arrays = append(c.arrays, typ)
		c.arrayByElem[element] = typ
		return nil
	}
	addFromType := func(goType types.Type, pos token.Pos) error {
		if isStringType(goType) {
			if err := addArray(types.Typ[types.Uint8], pos); err != nil {
				return err
			}
			c.stringArray = c.arrayByElem[types.Typ[types.Uint8]]
			return nil
		}
		if slice, ok := goType.Underlying().(*types.Slice); ok {
			return addArray(slice.Elem(), pos)
		}
		if pointer, ok := goType.Underlying().(*types.Pointer); ok {
			if array, ok := pointer.Elem().Underlying().(*types.Array); ok {
				return addArray(array.Elem(), pos)
			}
		}
		return nil
	}
	for _, fn := range c.functions {
		for i := 0; i < fn.Signature.Params().Len(); i++ {
			if err := addFromType(fn.Signature.Params().At(i).Type(), fn.Pos()); err != nil {
				return err
			}
		}
		for i := 0; i < fn.Signature.Results().Len(); i++ {
			if err := addFromType(fn.Signature.Results().At(i).Type(), fn.Pos()); err != nil {
				return err
			}
		}
		for _, block := range fn.Blocks {
			for _, instruction := range block.Instrs {
				if value, ok := instruction.(ssa.Value); ok {
					if err := addFromType(value.Type(), value.Pos()); err != nil {
						return err
					}
				}
				for _, operand := range instruction.Operands(nil) {
					if operand != nil && *operand != nil {
						if err := addFromType((*operand).Type(), instruction.Pos()); err != nil {
							return err
						}
						if constValue, ok := (*operand).(*ssa.Const); ok && constValue.Value != nil && constValue.Value.Kind() == constant.String {
							text := constant.StringVal(constValue.Value)
							if text != "" {
								if _, ok := c.stringIDs[text]; !ok {
									c.stringIDs[text] = len(c.strings)
									c.strings = append(c.strings, text)
								}
							}
						}
					}
				}
				switch instruction := instruction.(type) {
				case *ssa.Alloc:
					array, ok := dereference(instruction.Type()).Underlying().(*types.Array)
					if ok {
						if err := addArray(array.Elem(), instruction.Pos()); err != nil {
							return err
						}
					}
				case *ssa.MakeSlice:
					slice := instruction.Type().Underlying().(*types.Slice)
					if err := addArray(slice.Elem(), instruction.Pos()); err != nil {
						return err
					}
				}
			}
		}
	}
	for _, global := range c.packageGlobals() {
		if err := addFromType(dereference(global.Type()), global.Pos()); err != nil {
			return err
		}
	}
	return nil
}

func (c *compiler) writeStringEqual(out *strings.Builder, array *arrayType) {
	fmt.Fprintf(out, "  (func $stringEqual (param $left_base (ref null $array%d)) (param $left_offset i32) (param $left_len i32)", array.id)
	fmt.Fprintf(out, " (param $right_base (ref null $array%d)) (param $right_offset i32) (param $right_len i32) (result i32)\n", array.id)
	out.WriteString("    (local $index i32)\n")
	out.WriteString("    (if (i32.ne (local.get $left_len) (local.get $right_len)) (then (return (i32.const 0))))\n")
	out.WriteString("    (loop $compare\n")
	out.WriteString("      (if (i32.eq (local.get $index) (local.get $left_len)) (then (return (i32.const 1))))\n")
	fmt.Fprintf(out, "      (if (i32.ne (array.get_u $array%d (ref.as_non_null (local.get $left_base)) (i32.add (local.get $left_offset) (local.get $index)))", array.id)
	fmt.Fprintf(out, " (array.get_u $array%d (ref.as_non_null (local.get $right_base)) (i32.add (local.get $right_offset) (local.get $index)))) (then (return (i32.const 0))))\n", array.id)
	out.WriteString("      (local.set $index (i32.add (local.get $index) (i32.const 1)))\n")
	out.WriteString("      (br $compare))\n")
	out.WriteString("    (unreachable)\n")
	out.WriteString("  )\n")
}

func (c *compiler) findStructs() error {
	var addType func(types.Type) error
	var addBox func(types.Type) error
	addType = func(goType types.Type) error {
		goType = dereference(goType)
		if _, ok := c.structByGo[goType]; ok {
			return nil
		}
		for existingType, typ := range c.structByGo {
			if types.Identical(goType, existingType) {
				c.structByGo[goType] = typ
				return nil
			}
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
				if !isI32ValueType(fieldType) {
					return fmt.Errorf("wasm-gc: unsupported field type %s.%s: %s", goType, underlying.Field(i).Name(), fieldType)
				}
				physicalIndex++
			}
			typ.fields = append(typ.fields, f)
		}
		return nil
	}
	addBox = func(valueType types.Type) error {
		if _, ok := c.boxByGo[valueType]; ok {
			return nil
		}
		for existingType, typ := range c.boxByGo {
			if types.Identical(valueType, existingType) {
				c.boxByGo[valueType] = typ
				return nil
			}
		}
		typ := &structType{
			id:     len(c.structs),
			goType: valueType,
		}
		c.structs = append(c.structs, typ)
		c.boxByGo[valueType] = typ
		field := field{
			goType:        valueType,
			virtualOffset: 0,
			physicalIndex: 0,
		}
		if pointer, ok := valueType.Underlying().(*types.Pointer); ok {
			targetType := pointer.Elem()
			if _, ok := targetType.Underlying().(*types.Struct); !ok {
				return fmt.Errorf("wasm-gc: boxed pointer does not point to a struct: %s", valueType)
			}
			if err := addType(targetType); err != nil {
				return err
			}
			field.pointer = true
			field.target = c.structByGo[targetType]
		} else if !isI32ValueType(valueType) {
			return fmt.Errorf("wasm-gc: unsupported boxed value type: %s", valueType)
		}
		typ.fields = append(typ.fields, field)
		return nil
	}

	for _, fn := range c.functions {
		for i := 0; i < fn.Signature.Params().Len(); i++ {
			paramType := fn.Signature.Params().At(i).Type()
			if _, ok := paramType.Underlying().(*types.Struct); ok {
				if err := addType(paramType); err != nil {
					return c.errorAt(fn.Pos(), "%v", err)
				}
			}
		}
		for i := 0; i < fn.Signature.Results().Len(); i++ {
			resultType := fn.Signature.Results().At(i).Type()
			if _, ok := resultType.Underlying().(*types.Struct); ok {
				if err := addType(resultType); err != nil {
					return c.errorAt(fn.Pos(), "%v", err)
				}
			}
		}
	}
	for _, fn := range c.functions {
		for _, block := range fn.Blocks {
			for _, instruction := range block.Instrs {
				if value, ok := instruction.(ssa.Value); ok {
					if _, ok := value.Type().Underlying().(*types.Struct); ok {
						if err := addType(value.Type()); err != nil {
							return c.errorAt(value.Pos(), "%v", err)
						}
					}
				}
				for _, operand := range instruction.Operands(nil) {
					if operand == nil || *operand == nil {
						continue
					}
					if _, ok := (*operand).Type().Underlying().(*types.Struct); ok {
						if err := addType((*operand).Type()); err != nil {
							return c.errorAt(instruction.Pos(), "%v", err)
						}
					}
				}
				if alloc, ok := instruction.(*ssa.Alloc); ok {
					valueType := dereference(alloc.Type())
					if _, ok := valueType.Underlying().(*types.Array); ok {
						continue
					}
					var err error
					if _, ok := valueType.Underlying().(*types.Struct); ok {
						err = addType(valueType)
					} else {
						err = addBox(valueType)
					}
					if err != nil {
						return c.errorAt(alloc.Pos(), "%v", err)
					}
				}
			}
		}
	}
	for _, global := range c.packageGlobals() {
		valueType := dereference(global.Type())
		if pointer, ok := valueType.Underlying().(*types.Pointer); ok {
			if _, ok := pointer.Elem().Underlying().(*types.Struct); !ok {
				return c.errorAt(global.Pos(), "global pointer %s does not point to a struct", global.Name())
			}
			if err := addType(pointer.Elem()); err != nil {
				return c.errorAt(global.Pos(), "%v", err)
			}
		} else if _, ok := valueType.Underlying().(*types.Struct); ok {
			if err := addType(valueType); err != nil {
				return c.errorAt(global.Pos(), "%v", err)
			}
		}
	}
	for _, array := range c.arrays {
		if _, ok := array.element.Underlying().(*types.Pointer); ok {
			if err := addBox(array.element); err != nil {
				return err
			}
			array.elementBox = c.boxByGo[array.element]
		} else if _, ok := array.element.Underlying().(*types.Struct); ok {
			if err := addType(array.element); err != nil {
				return err
			}
			array.elementStruct = c.structForType(array.element)
		}
	}
	return nil
}

func (c *compiler) findGlobals() error {
	for _, global := range c.packageGlobals() {
		valueType := dereference(global.Type())
		info := &globalInfo{
			id:        len(c.globals),
			valueType: valueType,
		}
		if pointer, ok := valueType.Underlying().(*types.Pointer); ok {
			info.base = c.structByGo[pointer.Elem()]
			if info.base == nil {
				return c.errorAt(global.Pos(), "managed type was not discovered for global %s", global.Name())
			}
		} else if slice, ok := valueType.Underlying().(*types.Slice); ok {
			info.array = c.arrayByElem[slice.Elem()]
			info.slice = true
			if info.array == nil {
				return c.errorAt(global.Pos(), "managed array type was not discovered for global %s", global.Name())
			}
		} else if isStringType(valueType) {
			info.array = c.stringArray
			info.string = true
			if info.array == nil {
				return c.errorAt(global.Pos(), "managed string type was not discovered for global %s", global.Name())
			}
		} else if _, ok := valueType.Underlying().(*types.Struct); ok {
			info.base = c.structForType(valueType)
			info.structValue = true
			if info.base == nil {
				return c.errorAt(global.Pos(), "managed struct type was not discovered for global %s", global.Name())
			}
		} else if !isScalar(valueType) {
			return c.errorAt(global.Pos(), "unsupported global type %s: %s", global.Name(), valueType)
		}
		c.globalBySSA[global] = info
		c.globals = append(c.globals, info)
	}
	return nil
}

func (c *compiler) packageGlobals() []*ssa.Global {
	packages := make(map[*ssa.Package]struct{})
	for _, fn := range c.functions {
		if fn.Pkg != nil {
			packages[fn.Pkg] = struct{}{}
		}
	}
	sortedPackages := make([]*ssa.Package, 0, len(packages))
	for pkg := range packages {
		sortedPackages = append(sortedPackages, pkg)
	}
	sort.Slice(sortedPackages, func(i, j int) bool {
		return sortedPackages[i].Pkg.Path() < sortedPackages[j].Pkg.Path()
	})

	var globals []*ssa.Global
	for _, pkg := range sortedPackages {
		for _, member := range pkg.Members {
			if global, ok := member.(*ssa.Global); ok {
				globals = append(globals, global)
			}
		}
	}
	sort.Slice(globals, func(i, j int) bool {
		left := globals[i].Pkg.Pkg.Path() + "." + globals[i].Name()
		right := globals[j].Pkg.Pkg.Path() + "." + globals[j].Name()
		return left < right
	})
	return globals
}

func (fc *functionCompiler) compile() (string, error) {
	if len(fc.fn.Blocks) == 0 {
		return "", fc.compiler.errorAt(fc.fn.Pos(), "function %s has no body", fc.fn.String())
	}

	bindings := fc.compiler.closureBindings[fc.fn]
	if len(bindings) != len(fc.fn.FreeVars) {
		if len(fc.fn.FreeVars) != 0 {
			return "", fc.compiler.errorAt(fc.fn.Pos(), "closure binding count does not match free variables")
		}
	} else {
		for i, freeVar := range fc.fn.FreeVars {
			info, ok := fc.compiler.knownValues[bindings[i]]
			if !ok {
				return "", fc.compiler.errorAt(freeVar.Pos(), "closure binding information is unavailable for %s", freeVar.Name())
			}
			if info.array != nil && !info.slice && !info.string {
				return "", fc.compiler.errorAt(freeVar.Pos(), "captured array variables are not supported")
			}
			fc.values[freeVar] = info
			fc.compiler.knownValues[freeVar] = info
		}
	}
	for _, param := range fc.fn.Params {
		info, err := fc.infoForType(param.Type())
		if err != nil {
			return "", fc.compiler.errorAt(param.Pos(), "%v", err)
		}
		fc.values[param] = info
		fc.compiler.knownValues[param] = info
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
			fc.compiler.knownValues[value] = info
		}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "  (func $fn%d", fc.compiler.functionIDs[fc.fn])
	for _, freeVar := range fc.fn.FreeVars {
		writeDeclaration(&out, "param", valueName(freeVar), fc.values[freeVar])
	}
	for _, param := range fc.fn.Params {
		writeDeclaration(&out, "param", valueName(param), fc.values[param])
	}
	for i := 0; i < fc.fn.Signature.Results().Len(); i++ {
		info, err := fc.infoForType(fc.fn.Signature.Results().At(i).Type())
		if err != nil {
			return "", fc.compiler.errorAt(fc.fn.Pos(), "%v", err)
		}
		for _, typ := range wasmValueTypes(info) {
			fmt.Fprintf(&out, " (result %s)", typ)
		}
	}
	out.WriteString("\n")

	for _, block := range fc.fn.Blocks {
		for _, instruction := range block.Instrs {
			value, ok := instruction.(ssa.Value)
			if !ok || isZeroTuple(value.Type()) {
				continue
			}
			if fc.values[value].closure != nil {
				continue
			}
			writeDeclaration(&out, "local", valueName(value), fc.values[value])
			if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL {
				source := fc.values[load.X]
				if source.array != nil && source.array.elementBox != nil {
					fmt.Fprintf(&out, "    (local %s_element (ref null $type%d))\n", valueName(value), source.array.elementBox.id)
				}
			}
			if call, ok := value.(*ssa.Call); ok {
				if builtin, ok := call.Call.Value.(*ssa.Builtin); ok {
					switch builtin.Name() {
					case "append":
						fmt.Fprintf(&out, "    (local %s_append_cap i32)\n", valueName(value))
						if fc.values[value].array.elementStruct != nil {
							fmt.Fprintf(&out, "    (local %s_append_index i32)\n", valueName(value))
							fmt.Fprintf(&out, "    (local %s_append_values (ref null $array%d))\n", valueName(value), fc.values[value].array.id)
							fmt.Fprintf(&out, "    (local %s_append_source (ref null $type%d))\n", valueName(value), fc.values[value].array.elementStruct.id)
							fmt.Fprintf(&out, "    (local %s_append_destination (ref null $type%d))\n", valueName(value), fc.values[value].array.elementStruct.id)
						}
					case "copy":
						if len(call.Call.Args) == 2 {
							destination, err := fc.info(call.Call.Args[0])
							if err != nil {
								return "", err
							}
							if destination.array != nil && destination.array.elementStruct != nil {
								fmt.Fprintf(&out, "    (local %s_copy_index i32)\n", valueName(value))
								fmt.Fprintf(&out, "    (local %s_copy_source (ref null $type%d))\n", valueName(value), destination.array.elementStruct.id)
								fmt.Fprintf(&out, "    (local %s_copy_destination (ref null $type%d))\n", valueName(value), destination.array.elementStruct.id)
							}
						}
					}
				}
			}
			if phi, ok := value.(*ssa.Phi); ok {
				writeDeclaration(&out, "local", phiTempName(phi), fc.values[value])
			}
		}
	}
	for _, block := range fc.fn.Blocks {
		for _, instruction := range block.Instrs {
			if instruction, ok := instruction.(*ssa.Go); ok {
				id := fc.compiler.goIDs[instruction]
				fmt.Fprintf(&out, "    (local $task%d_new (ref null $task%d))\n", id, id)
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
		expressions, err := fc.valueExpressions(edge, info)
		if err != nil {
			return err
		}
		names := valuePartNames(phiTempName(phi), info)
		for i := range names {
			fmt.Fprintf(out, "              (local.set %s %s)\n", names[i], expressions[i])
		}
	}
	for _, phi := range phis {
		info := fc.values[phi]
		names := valuePartNames(valueName(phi), info)
		tempNames := valuePartNames(phiTempName(phi), info)
		for i := range names {
			fmt.Fprintf(out, "              (local.set %s (local.get %s))\n", names[i], tempNames[i])
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
		valueType := dereference(value.Type())
		if array, ok := valueType.Underlying().(*types.Array); ok {
			typ := fc.compiler.arrayByElem[array.Elem()]
			if typ == nil {
				return valueInfo{}, fc.compiler.errorAt(value.Pos(), "managed array type was not discovered: %s", array.Elem())
			}
			return valueInfo{goType: value.Type(), array: typ, length: array.Len(), field: -1}, nil
		}
		base := fc.compiler.structByGo[valueType]
		field := -1
		if base == nil {
			base = fc.compiler.boxByGo[valueType]
			field = 0
		}
		if base == nil {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "unsupported allocation type: %s", value.Type())
		}
		return valueInfo{goType: value.Type(), base: base, field: field}, nil
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
	case *ssa.Field:
		source, err := fc.info(value.X)
		if err != nil {
			return valueInfo{}, err
		}
		if !source.structValue || source.base == nil {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "field value has no managed struct")
		}
		if value.Field < 0 || value.Field >= len(source.base.fields) {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "invalid field index %d", value.Field)
		}
		field := source.base.fields[value.Field]
		if field.pointer {
			return valueInfo{goType: value.Type(), base: field.target, field: -1}, nil
		}
		return valueInfo{goType: value.Type(), field: -1}, nil
	case *ssa.IndexAddr:
		source, err := fc.info(value.X)
		if err != nil {
			return valueInfo{}, err
		}
		if source.array == nil {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "index address has no managed array")
		}
		if source.array.elementStruct != nil {
			return valueInfo{
				goType:       value.Type(),
				base:         source.array.elementStruct,
				elementArray: source.array,
				field:        -1,
			}, nil
		}
		return valueInfo{goType: value.Type(), array: source.array, field: -1}, nil
	case *ssa.Index:
		source, err := fc.info(value.X)
		if err != nil {
			return valueInfo{}, err
		}
		if !source.string {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "index value only supports managed strings")
		}
		return valueInfo{goType: value.Type(), field: -1}, nil
	case *ssa.Slice:
		source, err := fc.info(value.X)
		if err != nil {
			return valueInfo{}, err
		}
		if source.array == nil {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "slice has no managed array")
		}
		if isStringType(value.Type()) {
			return valueInfo{goType: value.Type(), array: source.array, string: true, field: -1}, nil
		}
		return valueInfo{goType: value.Type(), array: source.array, slice: true, field: -1}, nil
	case *ssa.MakeSlice:
		slice := value.Type().Underlying().(*types.Slice)
		typ := fc.compiler.arrayByElem[slice.Elem()]
		if typ == nil {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "managed array type was not discovered: %s", slice.Elem())
		}
		return valueInfo{goType: value.Type(), array: typ, slice: true, field: -1}, nil
	case *ssa.MakeClosure:
		return valueInfo{goType: value.Type(), field: -1, closure: value}, nil
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
		if source.array != nil {
			if source.array.elementBox != nil {
				field := source.array.elementBox.fields[0]
				return valueInfo{goType: value.Type(), base: field.target, field: -1}, nil
			}
			return valueInfo{goType: value.Type(), field: -1}, nil
		}
		if source.field < 0 {
			if source.global != nil {
				if source.global.structValue {
					return valueInfo{goType: value.Type(), base: source.global.base, structValue: true, field: -1}, nil
				}
				if source.global.base != nil {
					return valueInfo{goType: value.Type(), base: source.global.base, field: -1}, nil
				}
				if source.global.array != nil {
					return valueInfo{
						goType: value.Type(),
						array:  source.global.array,
						slice:  source.global.slice,
						string: source.global.string,
						field:  -1,
					}, nil
				}
				return valueInfo{goType: value.Type(), field: -1}, nil
			}
			if _, ok := value.Type().Underlying().(*types.Struct); ok {
				result, err := fc.infoForType(value.Type())
				if err != nil {
					return valueInfo{}, fc.compiler.errorAt(value.Pos(), "%v", err)
				}
				if result.base != source.base {
					return valueInfo{}, fc.compiler.errorAt(value.Pos(), "struct load has incompatible managed base")
				}
				return result, nil
			}
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
		if info.array != nil {
			fmt.Fprintf(out, "    (local.set %s (array.new_default $array%d (i32.const %d)))\n", baseName(instruction), info.array.id, info.length)
		} else {
			fmt.Fprintf(out, "    (local.set %s (struct.new_default $type%d))\n", baseName(instruction), info.base.id)
		}
		fmt.Fprintf(out, "    (local.set %s (i32.const 0))\n", offsetName(instruction))
		out.WriteString("    (drop (call $suspend))\n")
	case *ssa.FieldAddr:
		source, err := fc.info(instruction.X)
		if err != nil {
			return err
		}
		field := source.base.fields[instruction.Field]
		base, offset := fc.pointerExpressions(instruction.X, source)
		fmt.Fprintf(out, "    (drop (ref.as_non_null %s))\n", base)
		fmt.Fprintf(out, "    (local.set %s %s)\n", baseName(instruction), base)
		fmt.Fprintf(out, "    (local.set %s (i32.add %s (i32.const %d)))\n", offsetName(instruction), offset, field.virtualOffset)
	case *ssa.Field:
		return fc.emitField(out, instruction)
	case *ssa.Store:
		return fc.emitStore(out, instruction)
	case *ssa.IndexAddr:
		return fc.emitIndexAddr(out, instruction)
	case *ssa.Index:
		return fc.emitStringIndex(out, instruction)
	case *ssa.Slice:
		return fc.emitSlice(out, instruction)
	case *ssa.MakeSlice:
		info := fc.values[instruction]
		length := fc.expression(instruction.Len)
		capacity := fc.expression(instruction.Cap)
		fmt.Fprintf(out, "    (if (i32.lt_s %s (i32.const 0)) (then (unreachable)))\n", length)
		fmt.Fprintf(out, "    (if (i32.lt_s %s (i32.const 0)) (then (unreachable)))\n", capacity)
		fmt.Fprintf(out, "    (if (i32.gt_u %s %s) (then (unreachable)))\n", length, capacity)
		fmt.Fprintf(out, "    (local.set %s (array.new_default $array%d %s))\n", baseName(instruction), info.array.id, capacity)
		fmt.Fprintf(out, "    (local.set %s (i32.const 0))\n", offsetName(instruction))
		fmt.Fprintf(out, "    (local.set %s %s)\n", lenName(instruction), length)
		fmt.Fprintf(out, "    (local.set %s %s)\n", capName(instruction), capacity)
		out.WriteString("    (drop (call $suspend))\n")
	case *ssa.MakeClosure:
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
		fmt.Fprintf(out, "    (local.set $task%d_new (struct.new $task%d (ref.null $task%d)", id, id, id)
		for _, value := range goCallValues(instruction) {
			info, err := fc.info(value)
			if err != nil {
				return err
			}
			expressions, err := fc.valueExpressions(value, info)
			if err != nil {
				return err
			}
			for _, expression := range expressions {
				fmt.Fprintf(out, " %s", expression)
			}
		}
		out.WriteString("))\n")
		fmt.Fprintf(out, "    (if (ref.is_null (global.get $tasks%d_tail))\n", id)
		fmt.Fprintf(out, "      (then (global.set $tasks%d_head (local.get $task%d_new)))\n", id, id)
		fmt.Fprintf(out, "      (else (struct.set $task%d 0 (ref.as_non_null (global.get $tasks%d_tail)) (local.get $task%d_new))))\n", id, id, id)
		fmt.Fprintf(out, "    (global.set $tasks%d_tail (local.get $task%d_new))\n", id, id)
		fmt.Fprintf(out, "    (call $scheduleTask (i32.const %d))\n", id)
	case *ssa.ChangeType:
		return fc.emitChangeType(out, instruction)
	case *ssa.Convert:
		source, err := fc.info(instruction.X)
		if err != nil {
			return err
		}
		if source.base != nil || source.array != nil || source.global != nil || fc.values[instruction].base != nil || fc.values[instruction].array != nil {
			return fc.compiler.errorAt(instruction.Pos(), "pointer conversions are not supported")
		}
		expression := fc.expression(instruction.X)
		if isUint8(instruction.Type()) {
			expression = "(i32.and " + expression + " (i32.const 255))"
		}
		fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), expression)
	case *ssa.Return:
		out.WriteString("    (return")
		for _, result := range instruction.Results {
			info, err := fc.info(result)
			if err != nil {
				return err
			}
			expressions, err := fc.valueExpressions(result, info)
			if err != nil {
				return err
			}
			for _, expression := range expressions {
				fmt.Fprintf(out, " %s", expression)
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
	if left.array != nil || right.array != nil {
		if left.array == nil || right.array == nil || left.array != right.array {
			return fc.compiler.errorAt(instruction.Pos(), "managed array comparison has incompatible bases")
		}
		if instruction.Op != token.EQL && instruction.Op != token.NEQ {
			return fc.compiler.errorAt(instruction.Pos(), "unsupported managed array comparison: %s", instruction.Op)
		}
		if left.string || right.string {
			if !left.string || !right.string {
				return fc.compiler.errorAt(instruction.Pos(), "string comparison has incompatible values")
			}
			leftExpressions, err := fc.valueExpressions(instruction.X, left)
			if err != nil {
				return err
			}
			rightExpressions, err := fc.valueExpressions(instruction.Y, right)
			if err != nil {
				return err
			}
			expression := fmt.Sprintf("(call $stringEqual %s %s %s %s %s %s)",
				leftExpressions[0], leftExpressions[1], leftExpressions[2],
				rightExpressions[0], rightExpressions[1], rightExpressions[2])
			if instruction.Op == token.NEQ {
				expression = "(i32.eqz " + expression + ")"
			}
			fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), expression)
			return nil
		}
		if left.slice || right.slice {
			leftNil := isNilConst(instruction.X)
			rightNil := isNilConst(instruction.Y)
			if !leftNil && !rightNil {
				return fc.compiler.errorAt(instruction.Pos(), "slices can only be compared with nil")
			}
			other := instruction.X
			otherInfo := left
			if leftNil {
				other = instruction.Y
				otherInfo = right
			}
			expressions, err := fc.valueExpressions(other, otherInfo)
			if err != nil {
				return err
			}
			expression := "(ref.is_null " + expressions[0] + ")"
			if instruction.Op == token.NEQ {
				expression = "(i32.eqz " + expression + ")"
			}
			fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), expression)
			return nil
		}
		leftExpressions, err := fc.valueExpressions(instruction.X, left)
		if err != nil {
			return err
		}
		rightExpressions, err := fc.valueExpressions(instruction.Y, right)
		if err != nil {
			return err
		}
		expression := fmt.Sprintf("(i32.and (ref.eq %s %s) (i32.eq %s %s))", leftExpressions[0], rightExpressions[0], leftExpressions[1], rightExpressions[1])
		if instruction.Op == token.NEQ {
			expression = "(i32.eqz " + expression + ")"
		}
		fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), expression)
		return nil
	}
	if left.structValue || right.structValue {
		return fc.compiler.errorAt(instruction.Pos(), "struct value comparisons are not supported")
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
	if left.global != nil || right.global != nil {
		return fc.compiler.errorAt(instruction.Pos(), "global address comparisons are not supported")
	}

	op, err := wasmBinOp(instruction.Op, instruction.X.Type())
	if err != nil {
		return fc.compiler.errorAt(instruction.Pos(), "%v", err)
	}

	expression := fmt.Sprintf("(%s %s %s)", op, fc.expression(instruction.X), fc.expression(instruction.Y))
	if isUint8(instruction.Type()) {
		expression = "(i32.and " + expression + " (i32.const 255))"
	}
	fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), expression)
	return nil
}

func isNilConst(value ssa.Value) bool {
	constant, ok := value.(*ssa.Const)
	return ok && constant.IsNil()
}

func (fc *functionCompiler) emitChangeType(out *strings.Builder, instruction *ssa.ChangeType) error {
	destination := fc.values[instruction]
	source, err := fc.info(instruction.X)
	if err != nil {
		return err
	}
	if destination.structValue {
		if !source.structValue {
			return fc.compiler.errorAt(instruction.Pos(), "struct change has incompatible managed type")
		}
		expressions, err := fc.valueExpressions(instruction.X, source)
		if err != nil {
			return err
		}
		if source.base == destination.base {
			fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), expressions[0])
			return nil
		}
		if len(source.base.fields) != len(destination.base.fields) {
			return fc.compiler.errorAt(instruction.Pos(), "struct change has incompatible field layout")
		}
		receiver := "(ref.as_non_null " + expressions[0] + ")"
		fmt.Fprintf(out, "    (local.set %s (struct.new $type%d", valueName(instruction), destination.base.id)
		for i, destinationField := range destination.base.fields {
			sourceField := source.base.fields[i]
			if sourceField.pointer != destinationField.pointer {
				return fc.compiler.errorAt(instruction.Pos(), "struct change has incompatible field layout")
			}
			fmt.Fprintf(out, " (struct.get $type%d %d %s)", source.base.id, sourceField.physicalIndex, receiver)
			if sourceField.pointer {
				fmt.Fprintf(out, " (struct.get $type%d %d %s)", source.base.id, sourceField.physicalIndex+1, receiver)
			}
		}
		out.WriteString("))\n")
		return nil
	}
	if destination.base != nil {
		if source.base != destination.base {
			return fc.compiler.errorAt(instruction.Pos(), "pointer change has incompatible managed base")
		}
		base, offset := fc.pointerExpressions(instruction.X, source)
		fmt.Fprintf(out, "    (local.set %s %s)\n", baseName(instruction), base)
		fmt.Fprintf(out, "    (local.set %s %s)\n", offsetName(instruction), offset)
		return nil
	}
	if destination.array != nil {
		if source.array != destination.array || source.slice != destination.slice || source.string != destination.string {
			return fc.compiler.errorAt(instruction.Pos(), "slice change has incompatible managed array")
		}
		expressions, err := fc.valueExpressions(instruction.X, source)
		if err != nil {
			return err
		}
		names := valuePartNames(valueName(instruction), destination)
		for i := range names {
			fmt.Fprintf(out, "    (local.set %s %s)\n", names[i], expressions[i])
		}
		return nil
	}
	if source.base != nil || source.array != nil || source.global != nil {
		return fc.compiler.errorAt(instruction.Pos(), "pointer change is not supported")
	}
	fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), fc.expression(instruction.X))
	return nil
}

func (fc *functionCompiler) emitIndexAddr(out *strings.Builder, instruction *ssa.IndexAddr) error {
	source, err := fc.info(instruction.X)
	if err != nil {
		return err
	}
	if source.array == nil {
		return fc.compiler.errorAt(instruction.Pos(), "index address has no managed array")
	}
	index := fc.expression(instruction.Index)
	length := fc.lengthExpression(instruction.X, source)
	sourceExpressions, err := fc.valueExpressions(instruction.X, source)
	if err != nil {
		return err
	}
	if !source.slice && !source.string {
		fmt.Fprintf(out, "    (drop (ref.as_non_null %s))\n", sourceExpressions[0])
	}
	fmt.Fprintf(out, "    (if (i32.ge_u %s %s) (then (unreachable)))\n", index, length)
	if source.array.elementStruct != nil {
		receiver := "(ref.as_non_null " + sourceExpressions[0] + ")"
		physicalIndex := physicalArrayIndexExpression(sourceExpressions[1], index, source.array)
		fmt.Fprintf(out, "    (local.set %s (array.get $array%d %s %s))\n", baseName(instruction), source.array.id, receiver, physicalIndex)
		fmt.Fprintf(out, "    (if (ref.is_null (local.get %s))\n", baseName(instruction))
		fmt.Fprintf(out, "      (then (local.set %s (struct.new_default $type%d))\n", baseName(instruction), source.array.elementStruct.id)
		fmt.Fprintf(out, "        (array.set $array%d %s %s (local.get %s))))\n", source.array.id, receiver, physicalIndex, baseName(instruction))
		fmt.Fprintf(out, "    (local.set %s (i32.const 0))\n", offsetName(instruction))
		return nil
	}
	fmt.Fprintf(out, "    (local.set %s %s)\n", baseName(instruction), sourceExpressions[0])
	fmt.Fprintf(out, "    (local.set %s (i32.add %s %s))\n", offsetName(instruction), sourceExpressions[1], scaleArrayIndex(index, source.array))
	return nil
}

func (fc *functionCompiler) emitStringIndex(out *strings.Builder, instruction *ssa.Index) error {
	source, err := fc.info(instruction.X)
	if err != nil {
		return err
	}
	if !source.string {
		return fc.compiler.errorAt(instruction.Pos(), "index value only supports managed strings")
	}
	index := fc.expression(instruction.Index)
	fmt.Fprintf(out, "    (if (i32.ge_u %s %s) (then (unreachable)))\n", index, fc.lengthExpression(instruction.X, source))
	expressions, err := fc.valueExpressions(instruction.X, source)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "    (local.set %s (array.get_u $array%d (ref.as_non_null %s) (i32.add %s %s)))\n", valueName(instruction), source.array.id, expressions[0], expressions[1], index)
	return nil
}

func (fc *functionCompiler) emitSlice(out *strings.Builder, instruction *ssa.Slice) error {
	source, err := fc.info(instruction.X)
	if err != nil {
		return err
	}
	if source.array == nil {
		return fc.compiler.errorAt(instruction.Pos(), "slice has no managed array")
	}
	sourceExpressions, err := fc.valueExpressions(instruction.X, source)
	if err != nil {
		return err
	}
	if !source.slice && !source.string {
		fmt.Fprintf(out, "    (drop (ref.as_non_null %s))\n", sourceExpressions[0])
	}
	low := "(i32.const 0)"
	if instruction.Low != nil {
		low = fc.expression(instruction.Low)
	}
	high := fc.lengthExpression(instruction.X, source)
	if instruction.High != nil {
		high = fc.expression(instruction.High)
	}
	if source.string {
		if instruction.Max != nil {
			return fc.compiler.errorAt(instruction.Pos(), "three-index slicing is not valid for strings")
		}
		fmt.Fprintf(out, "    (if (i32.gt_u %s %s) (then (unreachable)))\n", low, high)
		fmt.Fprintf(out, "    (if (i32.gt_u %s %s) (then (unreachable)))\n", high, fc.lengthExpression(instruction.X, source))
		fmt.Fprintf(out, "    (local.set %s %s)\n", baseName(instruction), sourceExpressions[0])
		fmt.Fprintf(out, "    (local.set %s (i32.add %s %s))\n", offsetName(instruction), sourceExpressions[1], low)
		fmt.Fprintf(out, "    (local.set %s (i32.sub %s %s))\n", lenName(instruction), high, low)
		return nil
	}
	max := fc.capacityExpression(instruction.X, source)
	if instruction.Max != nil {
		max = fc.expression(instruction.Max)
	}
	fmt.Fprintf(out, "    (if (i32.gt_u %s %s) (then (unreachable)))\n", low, high)
	fmt.Fprintf(out, "    (if (i32.gt_u %s %s) (then (unreachable)))\n", high, max)
	fmt.Fprintf(out, "    (if (i32.gt_u %s %s) (then (unreachable)))\n", max, fc.capacityExpression(instruction.X, source))
	fmt.Fprintf(out, "    (local.set %s %s)\n", baseName(instruction), sourceExpressions[0])
	fmt.Fprintf(out, "    (local.set %s (i32.add %s %s))\n", offsetName(instruction), sourceExpressions[1], scaleArrayIndex(low, source.array))
	fmt.Fprintf(out, "    (local.set %s (i32.sub %s %s))\n", lenName(instruction), high, low)
	fmt.Fprintf(out, "    (local.set %s (i32.sub %s %s))\n", capName(instruction), max, low)
	return nil
}

func (fc *functionCompiler) emitStore(out *strings.Builder, instruction *ssa.Store) error {
	address, err := fc.info(instruction.Addr)
	if err != nil {
		return err
	}
	if address.global != nil {
		if address.global.structValue {
			value, err := fc.info(instruction.Val)
			if err != nil {
				return err
			}
			if !value.structValue || value.base != address.global.base {
				return fc.compiler.errorAt(instruction.Pos(), "global struct store has incompatible managed value")
			}
			expressions, err := fc.valueExpressions(instruction.Val, value)
			if err != nil {
				return err
			}
			source := "(ref.as_non_null " + expressions[0] + ")"
			destination := fmt.Sprintf("(ref.as_non_null (global.get $global%d))", address.global.id)
			fmt.Fprintf(out, "    (drop %s)\n", source)
			fmt.Fprintf(out, "    (drop %s)\n", destination)
			for _, field := range address.global.base.fields {
				fmt.Fprintf(out, "    (struct.set $type%d %d %s (struct.get $type%d %d %s))\n",
					address.global.base.id, field.physicalIndex, destination,
					address.global.base.id, field.physicalIndex, source)
				if field.pointer {
					fmt.Fprintf(out, "    (struct.set $type%d %d %s (struct.get $type%d %d %s))\n",
						address.global.base.id, field.physicalIndex+1, destination,
						address.global.base.id, field.physicalIndex+1, source)
				}
			}
		} else if address.global.array != nil {
			value, err := fc.info(instruction.Val)
			if err != nil {
				return err
			}
			if value.array != address.global.array || value.slice != address.global.slice || value.string != address.global.string {
				return fc.compiler.errorAt(instruction.Pos(), "global aggregate store has incompatible managed array")
			}
			expressions, err := fc.valueExpressions(instruction.Val, value)
			if err != nil {
				return err
			}
			names := []string{"base", "offset", "len"}
			if address.global.slice {
				names = append(names, "cap")
			}
			for i, name := range names {
				fmt.Fprintf(out, "    (global.set $global%d_%s %s)\n", address.global.id, name, expressions[i])
			}
		} else if address.global.base != nil {
			value, err := fc.info(instruction.Val)
			if err != nil {
				return err
			}
			if value.base != address.global.base {
				return fc.compiler.errorAt(instruction.Pos(), "global pointer store has incompatible managed base")
			}
			base, offset := fc.pointerExpressions(instruction.Val, value)
			fmt.Fprintf(out, "    (global.set $global%d_base %s)\n", address.global.id, base)
			fmt.Fprintf(out, "    (global.set $global%d_offset %s)\n", address.global.id, offset)
		} else {
			fmt.Fprintf(out, "    (global.set $global%d %s)\n", address.global.id, fc.expression(instruction.Val))
		}
		return nil
	}
	if address.array != nil {
		if address.array.elementBox != nil {
			value, err := fc.info(instruction.Val)
			if err != nil {
				return err
			}
			field := address.array.elementBox.fields[0]
			if value.base != field.target {
				return fc.compiler.errorAt(instruction.Pos(), "pointer array store has incompatible managed base")
			}
			base, offset := fc.pointerExpressions(instruction.Val, value)
			receiver := fmt.Sprintf("(ref.as_non_null (local.get %s))", baseName(instruction.Addr))
			index := physicalArrayIndex(instruction.Addr, address.array)
			fmt.Fprintf(out, "    (if (ref.is_null %s)\n", base)
			fmt.Fprintf(out, "      (then (array.set $array%d %s %s (ref.null $type%d)))\n", address.array.id, receiver, index, address.array.elementBox.id)
			fmt.Fprintf(out, "      (else (array.set $array%d %s %s (struct.new $type%d %s %s))))\n", address.array.id, receiver, index, address.array.elementBox.id, base, offset)
			return nil
		}
		fmt.Fprintf(out, "    (array.set $array%d (ref.as_non_null (local.get %s)) %s %s)\n", address.array.id, baseName(instruction.Addr), physicalArrayIndex(instruction.Addr, address.array), fc.expression(instruction.Val))
		return nil
	}
	if address.base != nil && address.field < 0 {
		value, err := fc.info(instruction.Val)
		if err != nil {
			return err
		}
		if !value.structValue || value.base != address.base {
			return fc.compiler.errorAt(instruction.Pos(), "whole struct store has incompatible managed value")
		}
		destinationBase, _ := fc.pointerExpressions(instruction.Addr, address)
		destination := "(ref.as_non_null " + destinationBase + ")"
		valueExpressions, err := fc.valueExpressions(instruction.Val, value)
		if err != nil {
			return err
		}
		source := "(ref.as_non_null " + valueExpressions[0] + ")"
		fmt.Fprintf(out, "    (drop %s)\n", destination)
		fmt.Fprintf(out, "    (drop %s)\n", source)
		for _, field := range address.base.fields {
			fmt.Fprintf(out, "    (struct.set $type%d %d %s (struct.get $type%d %d %s))\n",
				address.base.id, field.physicalIndex, destination,
				address.base.id, field.physicalIndex, source)
			if field.pointer {
				fmt.Fprintf(out, "    (struct.set $type%d %d %s (struct.get $type%d %d %s))\n",
					address.base.id, field.physicalIndex+1, destination,
					address.base.id, field.physicalIndex+1, source)
			}
		}
		return nil
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

func (fc *functionCompiler) emitField(out *strings.Builder, instruction *ssa.Field) error {
	source, err := fc.info(instruction.X)
	if err != nil {
		return err
	}
	if !source.structValue || source.base == nil {
		return fc.compiler.errorAt(instruction.Pos(), "field value has no managed struct")
	}
	field := source.base.fields[instruction.Field]
	sourceExpressions, err := fc.valueExpressions(instruction.X, source)
	if err != nil {
		return err
	}
	receiver := "(ref.as_non_null " + sourceExpressions[0] + ")"
	if field.pointer {
		fmt.Fprintf(out, "    (local.set %s (struct.get $type%d %d %s))\n", baseName(instruction), source.base.id, field.physicalIndex, receiver)
		fmt.Fprintf(out, "    (local.set %s (struct.get $type%d %d %s))\n", offsetName(instruction), source.base.id, field.physicalIndex+1, receiver)
	} else {
		fmt.Fprintf(out, "    (local.set %s (struct.get $type%d %d %s))\n", valueName(instruction), source.base.id, field.physicalIndex, receiver)
	}
	return nil
}

func (fc *functionCompiler) emitLoad(out *strings.Builder, instruction *ssa.UnOp) error {
	source, err := fc.info(instruction.X)
	if err != nil {
		return err
	}
	if source.global != nil {
		if source.global.structValue {
			receiver := fmt.Sprintf("(ref.as_non_null (global.get $global%d))", source.global.id)
			fmt.Fprintf(out, "    (drop %s)\n", receiver)
			fmt.Fprintf(out, "    (local.set %s (struct.new $type%d", valueName(instruction), source.global.base.id)
			for _, field := range source.global.base.fields {
				fmt.Fprintf(out, " (struct.get $type%d %d %s)", source.global.base.id, field.physicalIndex, receiver)
				if field.pointer {
					fmt.Fprintf(out, " (struct.get $type%d %d %s)", source.global.base.id, field.physicalIndex+1, receiver)
				}
			}
			out.WriteString("))\n")
		} else if source.global.array != nil {
			fmt.Fprintf(out, "    (local.set %s (global.get $global%d_base))\n", baseName(instruction), source.global.id)
			fmt.Fprintf(out, "    (local.set %s (global.get $global%d_offset))\n", offsetName(instruction), source.global.id)
			fmt.Fprintf(out, "    (local.set %s (global.get $global%d_len))\n", lenName(instruction), source.global.id)
			if source.global.slice {
				fmt.Fprintf(out, "    (local.set %s (global.get $global%d_cap))\n", capName(instruction), source.global.id)
			}
		} else if source.global.base != nil {
			fmt.Fprintf(out, "    (local.set %s (global.get $global%d_base))\n", baseName(instruction), source.global.id)
			fmt.Fprintf(out, "    (local.set %s (global.get $global%d_offset))\n", offsetName(instruction), source.global.id)
		} else {
			fmt.Fprintf(out, "    (local.set %s (global.get $global%d))\n", valueName(instruction), source.global.id)
		}
		return nil
	}
	if source.array != nil {
		if source.array.elementBox != nil {
			element := valueName(instruction) + "_element"
			fmt.Fprintf(out, "    (local.set %s (array.get $array%d (ref.as_non_null (local.get %s)) %s))\n", element, source.array.id, baseName(instruction.X), physicalArrayIndex(instruction.X, source.array))
			field := source.array.elementBox.fields[0]
			fmt.Fprintf(out, "    (if (ref.is_null (local.get %s))\n", element)
			fmt.Fprintf(out, "      (then (local.set %s (ref.null $type%d)) (local.set %s (i32.const 0)))\n", baseName(instruction), field.target.id, offsetName(instruction))
			fmt.Fprintf(out, "      (else (local.set %s (struct.get $type%d 0 (ref.as_non_null (local.get %s))))\n", baseName(instruction), source.array.elementBox.id, element)
			fmt.Fprintf(out, "        (local.set %s (struct.get $type%d 1 (ref.as_non_null (local.get %s))))))\n", offsetName(instruction), source.array.elementBox.id, element)
			return nil
		}
		fmt.Fprintf(out, "    (local.set %s (%s $array%d (ref.as_non_null (local.get %s)) %s))\n", valueName(instruction), arrayGetOp(source.array), source.array.id, baseName(instruction.X), physicalArrayIndex(instruction.X, source.array))
		return nil
	}
	result := fc.values[instruction]
	if source.base != nil && source.field < 0 && result.structValue {
		sourceBase, _ := fc.pointerExpressions(instruction.X, source)
		receiver := "(ref.as_non_null " + sourceBase + ")"
		fmt.Fprintf(out, "    (drop %s)\n", receiver)
		fmt.Fprintf(out, "    (local.set %s (struct.new $type%d", valueName(instruction), source.base.id)
		for _, field := range source.base.fields {
			fmt.Fprintf(out, " (struct.get $type%d %d %s)", source.base.id, field.physicalIndex, receiver)
			if field.pointer {
				fmt.Fprintf(out, " (struct.get $type%d %d %s)", source.base.id, field.physicalIndex+1, receiver)
			}
		}
		out.WriteString("))\n")
		return nil
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
		case "append":
			return fc.emitAppend(out, instruction)
		case "copy":
			return fc.emitCopy(out, instruction)
		case "len", "cap":
			if len(instruction.Call.Args) != 1 {
				return fc.compiler.errorAt(instruction.Pos(), "invalid %s call", builtin.Name())
			}
			arg := instruction.Call.Args[0]
			info, err := fc.info(arg)
			if err != nil {
				return err
			}
			if info.array == nil {
				return fc.compiler.errorAt(instruction.Pos(), "%s only supports managed arrays and slices", builtin.Name())
			}
			expression := fc.lengthExpression(arg, info)
			if builtin.Name() == "cap" {
				expression = fc.capacityExpression(arg, info)
			}
			fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), expression)
			return nil
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

	var bindings []ssa.Value
	var callee *ssa.Function
	if closure, ok := instruction.Call.Value.(*ssa.MakeClosure); ok {
		callee = closure.Fn.(*ssa.Function)
		bindings = closure.Bindings
	} else {
		callee = instruction.Call.StaticCallee()
		if callee == nil {
			return fc.compiler.errorAt(instruction.Pos(), "indirect calls are not supported")
		}
	}
	var call strings.Builder
	fmt.Fprintf(&call, "(call $fn%d", fc.compiler.functionIDs[callee])
	arguments := append(append([]ssa.Value(nil), bindings...), instruction.Call.Args...)
	for _, arg := range arguments {
		info, err := fc.info(arg)
		if err != nil {
			return err
		}
		expressions, err := fc.valueExpressions(arg, info)
		if err != nil {
			return err
		}
		for _, expression := range expressions {
			fmt.Fprintf(&call, " %s", expression)
		}
	}
	call.WriteString(")")

	if isZeroTuple(instruction.Type()) {
		fmt.Fprintf(out, "    %s\n", call.String())
		return nil
	}
	info := fc.values[instruction]
	if info.base != nil || info.array != nil {
		fmt.Fprintf(out, "    %s\n", call.String())
		names := valuePartNames(valueName(instruction), info)
		for i := len(names) - 1; i >= 0; i-- {
			fmt.Fprintf(out, "    (local.set %s)\n", names[i])
		}
	} else {
		fmt.Fprintf(out, "    (local.set %s %s)\n", valueName(instruction), call.String())
	}
	return nil
}

func (fc *functionCompiler) emitCopy(out *strings.Builder, instruction *ssa.Call) error {
	if len(instruction.Call.Args) != 2 {
		return fc.compiler.errorAt(instruction.Pos(), "copy requires destination and source slices")
	}
	destinationValue := instruction.Call.Args[0]
	sourceValue := instruction.Call.Args[1]
	destination, err := fc.info(destinationValue)
	if err != nil {
		return err
	}
	source, err := fc.info(sourceValue)
	if err != nil {
		return err
	}
	if !destination.slice || destination.array == nil {
		return fc.compiler.errorAt(instruction.Pos(), "copy destination must be a managed slice")
	}
	compatibleSlice := source.slice && source.array == destination.array
	compatibleString := source.string &&
		source.array != nil &&
		isUint8(destination.array.element) &&
		wasmArrayElementType(source.array) == wasmArrayElementType(destination.array)
	if !compatibleSlice && !compatibleString {
		return fc.compiler.errorAt(instruction.Pos(), "copy source has incompatible managed element type")
	}
	destinationExpressions, err := fc.valueExpressions(destinationValue, destination)
	if err != nil {
		return err
	}
	sourceExpressions, err := fc.valueExpressions(sourceValue, source)
	if err != nil {
		return err
	}
	count := valueName(instruction)
	destinationLength := destinationExpressions[2]
	sourceLength := sourceExpressions[2]
	fmt.Fprintf(out, "    (local.set %s (select %s %s (i32.lt_u %s %s)))\n",
		count, destinationLength, sourceLength, destinationLength, sourceLength)
	if destination.array.elementStruct != nil {
		return fc.emitStructCopy(out, instruction, destination.array, destinationExpressions, sourceExpressions)
	}
	fmt.Fprintf(out, "    (if (local.get %s)\n", count)
	fmt.Fprintf(out, "      (then (array.copy $array%d $array%d %s %s\n",
		destination.array.id, source.array.id,
		"(ref.as_non_null "+destinationExpressions[0]+")",
		physicalArrayIndexExpression(destinationExpressions[1], "(i32.const 0)", destination.array))
	fmt.Fprintf(out, "        %s %s (local.get %s))))\n",
		"(ref.as_non_null "+sourceExpressions[0]+")",
		physicalArrayIndexExpression(sourceExpressions[1], "(i32.const 0)", source.array),
		count)
	return nil
}

func (fc *functionCompiler) emitStructCopy(out *strings.Builder, instruction *ssa.Call, array *arrayType, destination, source []string) error {
	count := valueName(instruction)
	index := count + "_copy_index"
	sourceElement := count + "_copy_source"
	destinationElement := count + "_copy_destination"
	backwardLabel := count + "_copy_backward"
	forwardLabel := count + "_copy_forward"

	// Mutate existing slot objects so pointers to destination elements remain
	// valid, and copy backward when a later subslice overlaps its source.
	fmt.Fprintf(out, "    (if (local.get %s)\n", count)
	out.WriteString("      (then\n")
	fmt.Fprintf(out, "        (if (i32.and (ref.eq %s %s) (i32.gt_u %s %s))\n",
		destination[0], source[0], destination[1], source[1])
	out.WriteString("          (then\n")
	fmt.Fprintf(out, "            (local.set %s (local.get %s))\n", index, count)
	fmt.Fprintf(out, "            (loop %s\n", backwardLabel)
	fmt.Fprintf(out, "              (local.set %s (i32.sub (local.get %s) (i32.const 1)))\n", index, index)
	fc.writeStructCopyElement(out, array, index, sourceElement, destinationElement,
		destination[0], destination[1], source[0], source[1])
	fmt.Fprintf(out, "              (br_if %s (local.get %s)))\n", backwardLabel, index)
	out.WriteString("          )\n")
	out.WriteString("          (else\n")
	fmt.Fprintf(out, "            (local.set %s (i32.const 0))\n", index)
	fmt.Fprintf(out, "            (loop %s\n", forwardLabel)
	fc.writeStructCopyElement(out, array, index, sourceElement, destinationElement,
		destination[0], destination[1], source[0], source[1])
	fmt.Fprintf(out, "              (local.set %s (i32.add (local.get %s) (i32.const 1)))\n", index, index)
	fmt.Fprintf(out, "              (br_if %s (i32.lt_u (local.get %s) (local.get %s))))\n", forwardLabel, index, count)
	out.WriteString("          ))\n")
	out.WriteString("      ))\n")
	return nil
}

func (fc *functionCompiler) writeStructCopyElement(out *strings.Builder, array *arrayType, index, sourceElement, destinationElement, destinationBase, destinationOffset, sourceBase, sourceOffset string) {
	sourceIndex := physicalArrayIndexExpression(sourceOffset, "(local.get "+index+")", array)
	destinationIndex := physicalArrayIndexExpression(destinationOffset, "(local.get "+index+")", array)
	fmt.Fprintf(out, "              (local.set %s (array.get $array%d (ref.as_non_null %s) %s))\n", sourceElement, array.id, sourceBase, sourceIndex)
	fmt.Fprintf(out, "              (local.set %s (array.get $array%d (ref.as_non_null %s) %s))\n", destinationElement, array.id, destinationBase, destinationIndex)
	fc.writeStructElementAssignment(out, array, sourceElement, destinationElement,
		"(ref.as_non_null "+destinationBase+")", destinationIndex, true)
}

func (fc *functionCompiler) writeStructElementAssignment(out *strings.Builder, array *arrayType, sourceElement, destinationElement, destinationBase, destinationIndex string, cloneSource bool) {
	fmt.Fprintf(out, "              (if (ref.is_null (local.get %s))\n", destinationElement)
	out.WriteString("                (then\n")
	fmt.Fprintf(out, "                  (if (ref.is_null (local.get %s))\n", sourceElement)
	out.WriteString("                    (then)\n")
	if cloneSource {
		fmt.Fprintf(out, "                    (else (array.set $array%d %s %s (struct.new $type%d",
			array.id, destinationBase, destinationIndex, array.elementStruct.id)
		fc.writeStructFields(out, array.elementStruct, "(ref.as_non_null (local.get "+sourceElement+"))")
		out.WriteString("))))\n")
	} else {
		fmt.Fprintf(out, "                    (else (array.set $array%d %s %s (local.get %s))))\n",
			array.id, destinationBase, destinationIndex, sourceElement)
	}
	out.WriteString("                )\n")
	out.WriteString("                (else\n")
	fmt.Fprintf(out, "                  (if (ref.is_null (local.get %s))\n", sourceElement)
	out.WriteString("                    (then\n")
	fc.writeZeroStructFields(out, array.elementStruct, "(ref.as_non_null (local.get "+destinationElement+"))")
	out.WriteString("                    )\n")
	out.WriteString("                    (else\n")
	fc.writeCopyStructFields(out, array.elementStruct,
		"(ref.as_non_null (local.get "+destinationElement+"))",
		"(ref.as_non_null (local.get "+sourceElement+"))")
	out.WriteString("                    ))\n")
	out.WriteString("                ))\n")
}

func (fc *functionCompiler) emitAppend(out *strings.Builder, instruction *ssa.Call) error {
	if len(instruction.Call.Args) != 2 {
		return fc.compiler.errorAt(instruction.Pos(), "append requires a slice and variadic slice")
	}
	sourceValue := instruction.Call.Args[0]
	source, err := fc.info(sourceValue)
	if err != nil {
		return err
	}
	result := fc.values[instruction]
	if !source.slice || source.array == nil || result.array != source.array || !result.slice {
		return fc.compiler.errorAt(instruction.Pos(), "append only supports managed scalar slices")
	}
	addedValue := instruction.Call.Args[1]
	added, err := fc.info(addedValue)
	if err != nil {
		return err
	}
	compatibleSlice := added.slice && added.array == source.array
	compatibleString := added.string &&
		added.array != nil &&
		isUint8(source.array.element) &&
		wasmArrayElementType(added.array) == wasmArrayElementType(source.array)
	if !compatibleSlice && !compatibleString {
		return fc.compiler.errorAt(instruction.Pos(), "append variadic argument has incompatible slice type")
	}
	if source.array.elementStruct != nil {
		return fc.emitStructAppend(out, instruction, sourceValue, source, addedValue, added)
	}

	sourceExpressions, err := fc.valueExpressions(sourceValue, source)
	if err != nil {
		return err
	}
	addedExpressions, err := fc.valueExpressions(addedValue, added)
	if err != nil {
		return err
	}
	oldLength := sourceExpressions[2]
	oldCapacity := sourceExpressions[3]
	addedLength := addedExpressions[2]
	newLength := lenName(instruction)
	newCapacity := valueName(instruction) + "_append_cap"

	fmt.Fprintf(out, "    (local.set %s (i32.add %s %s))\n", newLength, oldLength, addedLength)
	fmt.Fprintf(out, "    (if (i32.lt_u (local.get %s) %s) (then (unreachable)))\n", newLength, oldLength)
	fmt.Fprintf(out, "    (if (i32.lt_s (local.get %s) (i32.const 0)) (then (unreachable)))\n", newLength)
	fmt.Fprintf(out, "    (if (i32.le_u (local.get %s) %s)\n", newLength, oldCapacity)
	out.WriteString("      (then\n")
	fmt.Fprintf(out, "        (local.set %s %s)\n", baseName(instruction), sourceExpressions[0])
	fmt.Fprintf(out, "        (local.set %s %s)\n", offsetName(instruction), sourceExpressions[1])
	fmt.Fprintf(out, "        (local.set %s %s))\n", capName(instruction), oldCapacity)
	out.WriteString("      (else\n")
	fmt.Fprintf(out, "        (local.set %s (i32.mul %s (i32.const 2)))\n", newCapacity, oldCapacity)
	fmt.Fprintf(out, "        (if (i32.or (i32.lt_s (local.get %s) (i32.const 0)) (i32.lt_u (local.get %s) (local.get %s)))\n", newCapacity, newCapacity, newLength)
	fmt.Fprintf(out, "          (then (local.set %s (local.get %s))))\n", newCapacity, newLength)
	fmt.Fprintf(out, "        (local.set %s (array.new_default $array%d (local.get %s)))\n", baseName(instruction), source.array.id, newCapacity)
	fmt.Fprintf(out, "        (local.set %s (i32.const 0))\n", offsetName(instruction))
	fmt.Fprintf(out, "        (local.set %s (local.get %s))\n", capName(instruction), newCapacity)
	out.WriteString("        (drop (call $suspend))\n")
	fmt.Fprintf(out, "        (if %s\n", oldLength)
	fmt.Fprintf(out, "          (then (array.copy $array%d $array%d (ref.as_non_null (local.get %s)) (i32.const 0)\n", source.array.id, source.array.id, baseName(instruction))
	fmt.Fprintf(out, "            (ref.as_non_null %s) %s %s)))))\n", sourceExpressions[0], physicalArrayIndexExpression(sourceExpressions[1], "(i32.const 0)", source.array), oldLength)
	fmt.Fprintf(out, "    (if %s\n", addedLength)
	fmt.Fprintf(out, "      (then (array.copy $array%d $array%d (ref.as_non_null (local.get %s))\n", source.array.id, added.array.id, baseName(instruction))
	fmt.Fprintf(out, "        %s (ref.as_non_null %s) %s %s)))\n",
		physicalArrayIndexExpression("(local.get "+offsetName(instruction)+")", oldLength, source.array),
		addedExpressions[0],
		physicalArrayIndexExpression(addedExpressions[1], "(i32.const 0)", source.array),
		addedLength)
	return nil
}

func (fc *functionCompiler) emitStructAppend(out *strings.Builder, instruction *ssa.Call, sourceValue ssa.Value, source valueInfo, addedValue ssa.Value, added valueInfo) error {
	sourceExpressions, err := fc.valueExpressions(sourceValue, source)
	if err != nil {
		return err
	}
	addedExpressions, err := fc.valueExpressions(addedValue, added)
	if err != nil {
		return err
	}
	array := source.array
	oldLength := sourceExpressions[2]
	oldCapacity := sourceExpressions[3]
	addedLength := addedExpressions[2]
	newLength := lenName(instruction)
	newCapacity := valueName(instruction) + "_append_cap"
	index := valueName(instruction) + "_append_index"
	values := valueName(instruction) + "_append_values"
	sourceElement := valueName(instruction) + "_append_source"
	destinationElement := valueName(instruction) + "_append_destination"

	fmt.Fprintf(out, "    (local.set %s (i32.add %s %s))\n", newLength, oldLength, addedLength)
	fmt.Fprintf(out, "    (if (i32.lt_u (local.get %s) %s) (then (unreachable)))\n", newLength, oldLength)
	fmt.Fprintf(out, "    (if (i32.lt_s (local.get %s) (i32.const 0)) (then (unreachable)))\n", newLength)

	// Snapshot before writing for overlap, then preserve existing slot objects when
	// capacity is reused so pointers to those elements remain valid.
	fmt.Fprintf(out, "    (if %s\n", addedLength)
	out.WriteString("      (then\n")
	fmt.Fprintf(out, "        (local.set %s (array.new_default $array%d %s))\n", values, array.id, addedLength)
	out.WriteString("        (drop (call $suspend))\n")
	fc.writeStructSnapshotLoop(out,
		valueName(instruction)+"_append_added_snapshot",
		array, index, sourceElement,
		"(ref.as_non_null (local.get "+values+"))", "(i32.const 0)",
		"(ref.as_non_null "+addedExpressions[0]+")", addedExpressions[1],
		addedLength)
	out.WriteString("      ))\n")

	fmt.Fprintf(out, "    (if (i32.le_u (local.get %s) %s)\n", newLength, oldCapacity)
	out.WriteString("      (then\n")
	fmt.Fprintf(out, "        (local.set %s %s)\n", baseName(instruction), sourceExpressions[0])
	fmt.Fprintf(out, "        (local.set %s %s)\n", offsetName(instruction), sourceExpressions[1])
	fmt.Fprintf(out, "        (local.set %s %s))\n", capName(instruction), oldCapacity)
	out.WriteString("      (else\n")
	fmt.Fprintf(out, "        (local.set %s (i32.mul %s (i32.const 2)))\n", newCapacity, oldCapacity)
	fmt.Fprintf(out, "        (if (i32.or (i32.lt_s (local.get %s) (i32.const 0)) (i32.lt_u (local.get %s) (local.get %s)))\n", newCapacity, newCapacity, newLength)
	fmt.Fprintf(out, "          (then (local.set %s (local.get %s))))\n", newCapacity, newLength)
	fmt.Fprintf(out, "        (local.set %s (array.new_default $array%d (local.get %s)))\n", baseName(instruction), array.id, newCapacity)
	fmt.Fprintf(out, "        (local.set %s (i32.const 0))\n", offsetName(instruction))
	fmt.Fprintf(out, "        (local.set %s (local.get %s))\n", capName(instruction), newCapacity)
	out.WriteString("        (drop (call $suspend))\n")
	fc.writeStructSnapshotLoop(out,
		valueName(instruction)+"_append_existing_snapshot",
		array, index, sourceElement,
		"(ref.as_non_null (local.get "+baseName(instruction)+"))", "(i32.const 0)",
		"(ref.as_non_null "+sourceExpressions[0]+")", sourceExpressions[1],
		oldLength)
	out.WriteString("      ))\n")

	fc.writeStructAssignmentLoop(out,
		valueName(instruction)+"_append_assign",
		array, index, sourceElement, destinationElement,
		"(ref.as_non_null (local.get "+baseName(instruction)+"))",
		"(i32.add (local.get "+offsetName(instruction)+") "+scaleArrayIndex(oldLength, array)+")",
		"(ref.as_non_null (local.get "+values+"))", "(i32.const 0)",
		addedLength)
	return nil
}

func (fc *functionCompiler) writeStructSnapshotLoop(out *strings.Builder, label string, array *arrayType, index, sourceElement, destinationBase, destinationOffset, sourceBase, sourceOffset, length string) {
	fmt.Fprintf(out, "        (local.set %s (i32.const 0))\n", index)
	fmt.Fprintf(out, "        (loop %s\n", label)
	fmt.Fprintf(out, "          (if (i32.lt_u (local.get %s) %s)\n", index, length)
	out.WriteString("            (then\n")
	fmt.Fprintf(out, "              (local.set %s (array.get $array%d %s %s))\n",
		sourceElement, array.id, sourceBase,
		physicalArrayIndexExpression(sourceOffset, "(local.get "+index+")", array))
	fmt.Fprintf(out, "              (if (ref.is_null (local.get %s))\n", sourceElement)
	out.WriteString("                (then)\n")
	fmt.Fprintf(out, "                (else (array.set $array%d %s %s (struct.new $type%d",
		array.id, destinationBase,
		physicalArrayIndexExpression(destinationOffset, "(local.get "+index+")", array),
		array.elementStruct.id)
	fc.writeStructFields(out, array.elementStruct, "(ref.as_non_null (local.get "+sourceElement+"))")
	out.WriteString("))))\n")
	fmt.Fprintf(out, "              (local.set %s (i32.add (local.get %s) (i32.const 1)))\n", index, index)
	fmt.Fprintf(out, "              (br %s))))\n", label)
}

func (fc *functionCompiler) writeStructAssignmentLoop(out *strings.Builder, label string, array *arrayType, index, sourceElement, destinationElement, destinationBase, destinationOffset, sourceBase, sourceOffset, length string) {
	fmt.Fprintf(out, "    (if %s\n", length)
	out.WriteString("      (then\n")
	fmt.Fprintf(out, "        (local.set %s (i32.const 0))\n", index)
	fmt.Fprintf(out, "        (loop %s\n", label)
	fmt.Fprintf(out, "          (if (i32.lt_u (local.get %s) %s)\n", index, length)
	out.WriteString("            (then\n")
	sourceIndex := physicalArrayIndexExpression(sourceOffset, "(local.get "+index+")", array)
	destinationIndex := physicalArrayIndexExpression(destinationOffset, "(local.get "+index+")", array)
	fmt.Fprintf(out, "              (local.set %s (array.get $array%d %s %s))\n", sourceElement, array.id, sourceBase, sourceIndex)
	fmt.Fprintf(out, "              (local.set %s (array.get $array%d %s %s))\n", destinationElement, array.id, destinationBase, destinationIndex)
	fc.writeStructElementAssignment(out, array, sourceElement, destinationElement,
		destinationBase, destinationIndex, false)
	fmt.Fprintf(out, "              (local.set %s (i32.add (local.get %s) (i32.const 1)))\n", index, index)
	fmt.Fprintf(out, "              (br %s))))\n", label)
	out.WriteString("        ))\n")
}

func (fc *functionCompiler) writeStructFields(out *strings.Builder, typ *structType, receiver string) {
	for _, field := range typ.fields {
		fmt.Fprintf(out, " (struct.get $type%d %d %s)", typ.id, field.physicalIndex, receiver)
		if field.pointer {
			fmt.Fprintf(out, " (struct.get $type%d %d %s)", typ.id, field.physicalIndex+1, receiver)
		}
	}
}

func (fc *functionCompiler) writeZeroStructFields(out *strings.Builder, typ *structType, destination string) {
	for _, field := range typ.fields {
		if field.pointer {
			fmt.Fprintf(out, "                      (struct.set $type%d %d %s (ref.null $type%d))\n", typ.id, field.physicalIndex, destination, field.target.id)
			fmt.Fprintf(out, "                      (struct.set $type%d %d %s (i32.const 0))\n", typ.id, field.physicalIndex+1, destination)
		} else {
			fmt.Fprintf(out, "                      (struct.set $type%d %d %s (i32.const 0))\n", typ.id, field.physicalIndex, destination)
		}
	}
}

func (fc *functionCompiler) writeCopyStructFields(out *strings.Builder, typ *structType, destination, source string) {
	for _, field := range typ.fields {
		fmt.Fprintf(out, "                      (struct.set $type%d %d %s (struct.get $type%d %d %s))\n",
			typ.id, field.physicalIndex, destination,
			typ.id, field.physicalIndex, source)
		if field.pointer {
			fmt.Fprintf(out, "                      (struct.set $type%d %d %s (struct.get $type%d %d %s))\n",
				typ.id, field.physicalIndex+1, destination,
				typ.id, field.physicalIndex+1, source)
		}
	}
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
	if info.global != nil && info.global.structValue {
		return fmt.Sprintf("(global.get $global%d)", info.global.id), "(i32.const 0)"
	}
	return "(local.get " + baseName(value) + ")", "(local.get " + offsetName(value) + ")"
}

func (fc *functionCompiler) valueExpressions(value ssa.Value, info valueInfo) ([]string, error) {
	if info.structValue {
		if constant, ok := value.(*ssa.Const); ok && constant.Value == nil {
			return []string{fmt.Sprintf("(struct.new_default $type%d)", info.base.id)}, nil
		}
		return []string{"(local.get " + valueName(value) + ")"}, nil
	}
	if info.array != nil {
		if constant, ok := value.(*ssa.Const); ok && constant.IsNil() {
			expressions := []string{fmt.Sprintf("(ref.null $array%d)", info.array.id), "(i32.const 0)"}
			if info.slice || info.string {
				expressions = append(expressions, "(i32.const 0)")
			}
			if info.slice {
				expressions = append(expressions, "(i32.const 0)")
			}
			return expressions, nil
		}
		if constValue, ok := value.(*ssa.Const); ok && info.string && constValue.Value != nil && constValue.Value.Kind() == constant.String {
			text := constant.StringVal(constValue.Value)
			if text == "" {
				return []string{fmt.Sprintf("(ref.null $array%d)", info.array.id), "(i32.const 0)", "(i32.const 0)"}, nil
			}
			id, ok := fc.compiler.stringIDs[text]
			if !ok {
				return nil, fc.compiler.errorAt(value.Pos(), "string constant was not discovered")
			}
			return []string{fmt.Sprintf("(global.get $string%d)", id), "(i32.const 0)", fmt.Sprintf("(i32.const %d)", len(text))}, nil
		}
		expressions := []string{"(local.get " + baseName(value) + ")", "(local.get " + offsetName(value) + ")"}
		if info.slice || info.string {
			expressions = append(expressions, "(local.get "+lenName(value)+")")
		}
		if info.slice {
			expressions = append(expressions, "(local.get "+capName(value)+")")
		}
		return expressions, nil
	}
	if info.base != nil {
		base, offset := fc.pointerExpressions(value, info)
		return []string{base, offset}, nil
	}
	if info.global != nil {
		return nil, fc.compiler.errorAt(value.Pos(), "global addresses do not have a first-class representation")
	}
	return []string{fc.expression(value)}, nil
}

func (fc *functionCompiler) lengthExpression(value ssa.Value, info valueInfo) string {
	if constValue, ok := value.(*ssa.Const); ok && info.string && constValue.Value != nil && constValue.Value.Kind() == constant.String {
		return "(i32.const " + strconv.Itoa(len(constant.StringVal(constValue.Value))) + ")"
	}
	if constant, ok := value.(*ssa.Const); ok && constant.IsNil() {
		return "(i32.const 0)"
	}
	if info.slice || info.string {
		return "(local.get " + lenName(value) + ")"
	}
	return "(i32.const " + strconv.FormatInt(info.length, 10) + ")"
}

func (fc *functionCompiler) capacityExpression(value ssa.Value, info valueInfo) string {
	if constant, ok := value.(*ssa.Const); ok && constant.IsNil() {
		return "(i32.const 0)"
	}
	if info.slice {
		return "(local.get " + capName(value) + ")"
	}
	return "(i32.const " + strconv.FormatInt(info.length, 10) + ")"
}

func scaleArrayIndex(index string, array *arrayType) string {
	if array.elementSize == 1 {
		return index
	}
	return fmt.Sprintf("(i32.mul %s (i32.const %d))", index, array.elementSize)
}

func physicalArrayIndex(value ssa.Value, array *arrayType) string {
	offset := "(local.get " + offsetName(value) + ")"
	if array.elementSize == 1 {
		return offset
	}
	return fmt.Sprintf("(i32.div_u %s (i32.const %d))", offset, array.elementSize)
}

func physicalArrayIndexExpression(offset, index string, array *arrayType) string {
	if array.elementSize != 1 {
		offset = fmt.Sprintf("(i32.div_u %s (i32.const %d))", offset, array.elementSize)
	}
	return fmt.Sprintf("(i32.add %s %s)", offset, index)
}

func (fc *functionCompiler) info(value ssa.Value) (valueInfo, error) {
	if constant, ok := value.(*ssa.Const); ok {
		return fc.infoForType(constant.Type())
	}
	if global, ok := value.(*ssa.Global); ok {
		info := fc.compiler.globalBySSA[global]
		if info == nil {
			return valueInfo{}, fc.compiler.errorAt(value.Pos(), "missing global information for %s", value.Name())
		}
		valueInfo := valueInfo{goType: value.Type(), field: -1, global: info}
		if info.structValue {
			valueInfo.base = info.base
		}
		return valueInfo, nil
	}
	info, ok := fc.values[value]
	if !ok {
		return valueInfo{}, fc.compiler.errorAt(value.Pos(), "missing value information for %s", value.Name())
	}
	return info, nil
}

func (c *compiler) info(value ssa.Value) (valueInfo, error) {
	if constant, ok := value.(*ssa.Const); ok {
		return c.infoForType(constant.Type())
	}
	if global, ok := value.(*ssa.Global); ok {
		info := c.globalBySSA[global]
		if info == nil {
			return valueInfo{}, c.errorAt(value.Pos(), "missing global information for %s", value.Name())
		}
		valueInfo := valueInfo{goType: value.Type(), field: -1, global: info}
		if info.structValue {
			valueInfo.base = info.base
		}
		return valueInfo, nil
	}
	info, ok := c.knownValues[value]
	if !ok {
		return valueInfo{}, c.errorAt(value.Pos(), "missing value information for %s", value.Name())
	}
	return info, nil
}

func (c *compiler) infoForType(goType types.Type) (valueInfo, error) {
	return (&functionCompiler{compiler: c}).infoForType(goType)
}

func (c *compiler) structForType(goType types.Type) *structType {
	if typ := c.structByGo[goType]; typ != nil {
		return typ
	}
	for existingType, typ := range c.structByGo {
		if types.Identical(goType, existingType) {
			c.structByGo[goType] = typ
			return typ
		}
	}
	return nil
}

func (fc *functionCompiler) infoForType(goType types.Type) (valueInfo, error) {
	if isStringType(goType) {
		if fc.compiler.stringArray == nil {
			return valueInfo{}, fmt.Errorf("wasm-gc: managed string array type was not discovered")
		}
		return valueInfo{goType: goType, array: fc.compiler.stringArray, string: true, field: -1}, nil
	}
	if slice, ok := goType.Underlying().(*types.Slice); ok {
		array := fc.compiler.arrayByElem[slice.Elem()]
		if array == nil {
			return valueInfo{}, fmt.Errorf("wasm-gc: managed array type was not discovered: %s", slice.Elem())
		}
		return valueInfo{goType: goType, array: array, slice: true, field: -1}, nil
	}
	if pointer, ok := goType.Underlying().(*types.Pointer); ok {
		target := dereference(pointer)
		if array, ok := target.Underlying().(*types.Array); ok {
			managedArray := fc.compiler.arrayByElem[array.Elem()]
			if managedArray == nil {
				return valueInfo{}, fmt.Errorf("wasm-gc: managed array type was not discovered: %s", array.Elem())
			}
			return valueInfo{goType: goType, array: managedArray, length: array.Len(), field: -1}, nil
		}
		if _, ok := target.Underlying().(*types.Struct); !ok {
			return valueInfo{}, fmt.Errorf("wasm-gc: pointer type has no statically known managed base: %s", goType)
		}
		base := fc.compiler.structForType(target)
		if base == nil {
			return valueInfo{}, fmt.Errorf("wasm-gc: managed type was not discovered: %s", target)
		}
		return valueInfo{goType: goType, base: base, field: -1}, nil
	}
	if _, ok := goType.Underlying().(*types.Struct); ok {
		base := fc.compiler.structForType(goType)
		if base == nil {
			return valueInfo{}, fmt.Errorf("wasm-gc: managed struct value type was not discovered: %s", goType)
		}
		return valueInfo{goType: goType, base: base, structValue: true, field: -1}, nil
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
		types.Uint, types.Uint8, types.Uint32, types.Uintptr:
		return true
	default:
		return false
	}
}

func isI32ValueType(goType types.Type) bool {
	if _, ok := goType.Underlying().(*types.Chan); ok {
		return true
	}
	return isScalar(goType)
}

func isUint8(goType types.Type) bool {
	basic, ok := goType.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

func isStringType(goType types.Type) bool {
	basic, ok := goType.Underlying().(*types.Basic)
	return ok && (basic.Kind() == types.String || basic.Kind() == types.UntypedString)
}

func wasmArrayElementType(array *arrayType) string {
	if array.elementBox != nil {
		return fmt.Sprintf("(ref null $type%d)", array.elementBox.id)
	}
	if array.elementStruct != nil {
		return fmt.Sprintf("(ref null $type%d)", array.elementStruct.id)
	}
	if basic, ok := array.element.Underlying().(*types.Basic); ok && basic.Kind() == types.Uint8 {
		return "i8"
	}
	return wasmScalarType(array.element)
}

func arrayGetOp(array *arrayType) string {
	if wasmArrayElementType(array) == "i8" {
		return "array.get_u"
	}
	return "array.get"
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

func lenName(value ssa.Value) string {
	return valueName(value) + "_len"
}

func capName(value ssa.Value) string {
	return valueName(value) + "_cap"
}

func phiTempName(phi *ssa.Phi) string {
	return valueName(phi) + "_phi"
}

func writeDeclaration(out *strings.Builder, kind, name string, info valueInfo) {
	if info.structValue {
		fmt.Fprintf(out, " (%s %s (ref null $type%d))", kind, name, info.base.id)
	} else if info.array != nil {
		fmt.Fprintf(out, " (%s %s_base (ref null $array%d))", kind, name, info.array.id)
		fmt.Fprintf(out, " (%s %s_offset i32)", kind, name)
		if info.slice || info.string {
			fmt.Fprintf(out, " (%s %s_len i32)", kind, name)
		}
		if info.slice {
			fmt.Fprintf(out, " (%s %s_cap i32)", kind, name)
		}
	} else if info.base != nil {
		fmt.Fprintf(out, " (%s %s_base (ref null $type%d))", kind, name, info.base.id)
		fmt.Fprintf(out, " (%s %s_offset i32)", kind, name)
	} else {
		fmt.Fprintf(out, " (%s %s %s)", kind, name, wasmScalarType(info.goType))
	}
}

func wasmValueTypes(info valueInfo) []string {
	if info.structValue {
		return []string{fmt.Sprintf("(ref null $type%d)", info.base.id)}
	}
	if info.array != nil {
		types := []string{fmt.Sprintf("(ref null $array%d)", info.array.id), "i32"}
		if info.slice || info.string {
			types = append(types, "i32")
		}
		if info.slice {
			types = append(types, "i32")
		}
		return types
	}
	if info.base != nil {
		return []string{fmt.Sprintf("(ref null $type%d)", info.base.id), "i32"}
	}
	return []string{wasmScalarType(info.goType)}
}

func valuePartNames(name string, info valueInfo) []string {
	if info.structValue {
		return []string{name}
	}
	if info.array != nil {
		names := []string{name + "_base", name + "_offset"}
		if info.slice || info.string {
			names = append(names, name+"_len")
		}
		if info.slice {
			names = append(names, name+"_cap")
		}
		return names
	}
	if info.base != nil {
		return []string{name + "_base", name + "_offset"}
	}
	return []string{name}
}

func isZeroTuple(goType types.Type) bool {
	tuple, ok := goType.(*types.Tuple)
	return ok && tuple.Len() == 0
}
