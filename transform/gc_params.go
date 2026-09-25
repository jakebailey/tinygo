package transform

import (
	"strings"

	"tinygo.org/x/go-llvm"
)

func gcParameterRoots(fn llvm.Value, trackFuncs, trackPointers map[llvm.Value]struct{}) map[llvm.Value][]llvm.Value {
	var safepoints []llvm.Value
	for bb := fn.FirstBasicBlock(); !bb.IsNil(); bb = llvm.NextBasicBlock(bb) {
		for inst := bb.FirstInstruction(); !inst.IsNil(); inst = llvm.NextInstruction(inst) {
			if inst.IsACallInst().IsNil() {
				continue
			}
			if gcCanCollect(inst, trackFuncs, trackPointers) {
				safepoints = append(safepoints, inst)
			}
		}
	}
	if len(safepoints) == 0 {
		return nil
	}

	roots := make(map[llvm.Value][]llvm.Value)
	for param := fn.FirstParam(); !param.IsNil(); param = llvm.NextParam(param) {
		if param.Type().TypeKind() != llvm.PointerTypeKind {
			continue
		}
		derived := map[llvm.Value]struct{}{param: {}}
		for work := []llvm.Value{param}; len(work) != 0; {
			value := work[len(work)-1]
			work = work[:len(work)-1]
			for use := value.FirstUse(); !use.IsNil(); use = use.NextUse() {
				user := use.User()
				if user.IsAInstruction().IsNil() {
					continue
				}
				switch user.InstructionOpcode() {
				case llvm.GetElementPtr, llvm.PHI, llvm.Select, llvm.InsertValue, llvm.ExtractValue:
					if _, ok := derived[user]; !ok {
						derived[user] = struct{}{}
						work = append(work, user)
					}
				default:
					if !user.IsACastInst().IsNil() && user.Type().TypeKind() == llvm.PointerTypeKind {
						if _, ok := derived[user]; !ok {
							derived[user] = struct{}{}
							work = append(work, user)
						}
					}
				}
			}
		}

		uses := make(map[llvm.Value]struct{})
		var dataPointer bool
		for value := range derived {
			for use := value.FirstUse(); !use.IsNil(); use = use.NextUse() {
				user := use.User()
				if user.IsAInstruction().IsNil() || !user.IsADbgInfoIntrinsic().IsNil() {
					continue
				}
				if !user.IsACallInst().IsNil() {
					if _, ok := trackPointers[user.CalledValue()]; ok {
						continue
					}
				}
				uses[user] = struct{}{}
				if user.InstructionOpcode() == llvm.GetElementPtr ||
					user.InstructionOpcode() == llvm.Load ||
					user.InstructionOpcode() == llvm.Store ||
					(!user.IsACallInst().IsNil() && user.CalledValue() != value) {
					dataPointer = true
				}
			}
		}
		if !dataPointer {
			continue
		}
		for _, call := range safepoints {
			_, usedDuringCall := uses[call]
			if usedDuringCall || gcHasFutureUse(call, uses, llvm.Value{}) {
				roots[call] = append(roots[call], param)
			}
		}
	}
	return roots
}

func gcCanCollect(call llvm.Value, trackFuncs, trackPointers map[llvm.Value]struct{}) bool {
	callee := call.CalledValue()
	if _, ok := trackPointers[callee]; ok || !callee.IsAInlineAsm().IsNil() {
		return false
	}
	if _, ok := trackFuncs[callee]; ok {
		return true
	}
	if callee.IsAFunction().IsNil() {
		return true
	}
	if !callee.IsDeclaration() || strings.HasPrefix(callee.Name(), "llvm.") {
		return false
	}
	mem := callee.GetEnumFunctionAttribute(llvm.AttributeKindID("memory"))
	return mem.IsNil() || mem.GetEnumValue()>>shiftExcludeArgMem != 0
}

func gcHasFutureUse(call llvm.Value, uses map[llvm.Value]struct{}, definition llvm.Value) bool {
	type location struct {
		block llvm.BasicBlock
		first llvm.Value
	}
	work := []location{{call.InstructionParent(), llvm.NextInstruction(call)}}
	visited := make(map[llvm.BasicBlock]struct{})
	for len(work) != 0 {
		loc := work[len(work)-1]
		work = work[:len(work)-1]
		if loc.first == loc.block.FirstInstruction() {
			if _, ok := visited[loc.block]; ok {
				continue
			}
			visited[loc.block] = struct{}{}
		}
		var redefined bool
		for inst := loc.first; !inst.IsNil(); inst = llvm.NextInstruction(inst) {
			if inst == definition {
				redefined = true
				break
			}
			if _, ok := uses[inst]; ok {
				return true
			}
		}
		if redefined {
			continue
		}
		term := loc.block.LastInstruction()
		for i := range term.SuccessorsCount() {
			next := term.Successor(i)
			work = append(work, location{next, next.FirstInstruction()})
		}
	}
	return false
}

func gcRootLiveAfter(ptr, call llvm.Value) bool {
	uses := make(map[llvm.Value]struct{})
	derived := map[llvm.Value]struct{}{ptr: {}}
	for work := []llvm.Value{ptr}; len(work) != 0; {
		value := work[len(work)-1]
		work = work[:len(work)-1]
		for use := value.FirstUse(); !use.IsNil(); use = use.NextUse() {
			user := use.User()
			if user.IsAInstruction().IsNil() || !user.IsADbgInfoIntrinsic().IsNil() {
				continue
			}
			if user.InstructionOpcode() == llvm.PHI {
				// A PHI reads its input on the incoming edge, not in its block.
				// See https://llvm.org/docs/LangRef.html#phi-instruction.
				return true
			}
			if (user.Type().TypeKind() == llvm.PointerTypeKind ||
				user.Type().TypeKind() == llvm.StructTypeKind ||
				user.Type().TypeKind() == llvm.ArrayTypeKind) &&
				(user.InstructionOpcode() == llvm.GetElementPtr ||
					user.InstructionOpcode() == llvm.Select ||
					user.InstructionOpcode() == llvm.InsertValue ||
					user.InstructionOpcode() == llvm.ExtractValue ||
					!user.IsACastInst().IsNil()) {
				if _, ok := derived[user]; !ok {
					derived[user] = struct{}{}
					work = append(work, user)
				}
			}
			uses[user] = struct{}{}
		}
	}

	if _, usedDuringCall := uses[call]; usedDuringCall {
		return true
	}
	return gcHasFutureUse(call, uses, ptr)
}

func gcRootDeadCalls(ptr llvm.Value, trackFuncs, trackPointers map[llvm.Value]struct{}) (bool, []llvm.Value) {
	type location struct {
		block llvm.BasicBlock
		first llvm.Value
	}
	work := []location{{ptr.InstructionParent(), llvm.NextInstruction(ptr)}}
	visited := make(map[llvm.BasicBlock]struct{})
	var live bool
	var deadCalls []llvm.Value
	for len(work) != 0 {
		loc := work[len(work)-1]
		work = work[:len(work)-1]
		if loc.first == loc.block.FirstInstruction() {
			if _, ok := visited[loc.block]; ok {
				continue
			}
			visited[loc.block] = struct{}{}
		}
		var stop bool
		for inst := loc.first; !inst.IsNil(); inst = llvm.NextInstruction(inst) {
			if inst == ptr {
				stop = true
				break
			}
			if inst.IsACallInst().IsNil() || !gcCanCollect(inst, trackFuncs, trackPointers) {
				continue
			}
			if !gcRootLiveAfter(ptr, inst) {
				deadCalls = append(deadCalls, inst)
				stop = true
				break
			}
			live = true
		}
		if stop {
			continue
		}
		term := loc.block.LastInstruction()
		for i := range term.SuccessorsCount() {
			next := term.Successor(i)
			work = append(work, location{next, next.FirstInstruction()})
		}
	}
	return live, deadCalls
}

func gcRootReturned(ptr llvm.Value) bool {
	derived := map[llvm.Value]struct{}{ptr: {}}
	for work := []llvm.Value{ptr}; len(work) != 0; {
		value := work[len(work)-1]
		work = work[:len(work)-1]
		for use := value.FirstUse(); !use.IsNil(); use = use.NextUse() {
			user := use.User()
			if user.IsAInstruction().IsNil() {
				continue
			}
			if user.InstructionOpcode() == llvm.Ret {
				return true
			}
			switch user.InstructionOpcode() {
			case llvm.InsertValue, llvm.ExtractValue, llvm.PHI, llvm.Select, llvm.BitCast, llvm.GetElementPtr:
				if _, ok := derived[user]; !ok {
					derived[user] = struct{}{}
					work = append(work, user)
				}
			}
		}
	}
	return false
}
