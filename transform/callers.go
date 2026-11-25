package transform

import (
	"github.com/tinygo-org/tinygo/compileopts"
	"tinygo.org/x/go-llvm"
)

func preserveCallerFrames(mod llvm.Module, config *compileopts.Config) {
	if config.Scheduler() != "threads" {
		return
	}
	switch config.GOOS() {
	case "linux", "darwin":
	default:
		return
	}
	switch config.GOARCH() {
	case "386", "amd64", "arm64":
	default:
		return
	}
	if mod.NamedFunction("runtime.Callers").IsNil() {
		return
	}

	callers := mod.NamedFunction("runtime.Callers")
	marked := map[llvm.Value]struct{}{
		callers: {},
	}
	worklist := make([]llvm.Value, 0)
	indirectTypes := make(map[llvm.Type]struct{})
	// Only direct callers supply indirect signatures. Expanding signatures
	// from inferred callers also marks unrelated callbacks with common types.
	for _, use := range getUses(callers) {
		if use.IsACallInst().IsNil() || use.CalledValue() != callers {
			continue
		}
		parent := use.InstructionParent().Parent()
		if _, ok := marked[parent]; !ok {
			marked[parent] = struct{}{}
			worklist = append(worklist, parent)
		}
		for _, parentUse := range getUses(parent) {
			if parentUse.IsACallInst().IsNil() && !parentUse.IsAInstruction().IsNil() {
				indirectTypes[parent.GlobalValueType()] = struct{}{}
			}
		}
	}

	for len(worklist) != 0 {
		fn := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		for _, use := range getUses(fn) {
			if use.IsACallInst().IsNil() || use.CalledValue() != fn {
				continue
			}
			parent := use.InstructionParent().Parent()
			if _, ok := marked[parent]; !ok {
				marked[parent] = struct{}{}
				worklist = append(worklist, parent)
			}
		}
	}

	for fn := mod.FirstFunction(); !fn.IsNil(); fn = llvm.NextFunction(fn) {
		for block := fn.FirstBasicBlock(); !block.IsNil(); block = llvm.NextBasicBlock(block) {
			for instruction := block.FirstInstruction(); !instruction.IsNil(); instruction = llvm.NextInstruction(instruction) {
				call := instruction.IsACallInst()
				if call.IsNil() || !call.CalledValue().IsAFunction().IsNil() {
					continue
				}
				if _, ok := indirectTypes[call.CalledFunctionType()]; !ok {
					continue
				}
				if _, ok := marked[fn]; !ok {
					marked[fn] = struct{}{}
					worklist = append(worklist, fn)
				}
			}
		}
	}

	for len(worklist) != 0 {
		fn := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		for _, use := range getUses(fn) {
			if use.IsACallInst().IsNil() || use.CalledValue() != fn {
				continue
			}
			parent := use.InstructionParent().Parent()
			if _, ok := marked[parent]; !ok {
				marked[parent] = struct{}{}
				worklist = append(worklist, parent)
			}
		}
	}

	ctx := mod.Context()
	alwaysInline := llvm.AttributeKindID("alwaysinline")
	noInline := ctx.CreateEnumAttribute(llvm.AttributeKindID("noinline"), 0)
	disableTailCalls := ctx.CreateStringAttribute("disable-tail-calls", "true")
	framePointer := ctx.CreateStringAttribute("frame-pointer", "all")
	for fn := range marked {
		if fn.IsDeclaration() {
			continue
		}
		fn.RemoveEnumFunctionAttribute(alwaysInline)
		fn.AddFunctionAttr(noInline)
		fn.AddFunctionAttr(disableTailCalls)
		fn.AddFunctionAttr(framePointer)
	}
}
