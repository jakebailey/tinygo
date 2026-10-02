package transform_test

import (
	"errors"
	"testing"

	"github.com/tinygo-org/tinygo/compiler/llvmutil"
	"github.com/tinygo-org/tinygo/transform"
	"tinygo.org/x/go-llvm"
)

func TestMakeGCStackSlots(t *testing.T) {
	t.Parallel()
	testTransform(t, "testdata/gc-stackslots", func(mod llvm.Module) {
		transform.MakeGCStackSlots(mod)
		for name, want := range map[string]int{
			"deadStackLifetime":      1,
			"restartedStackLifetime": 3,
		} {
			fn := mod.NamedFunction(name)
			var clears int
			for bb := fn.FirstBasicBlock(); !bb.IsNil(); bb = llvm.NextBasicBlock(bb) {
				for inst := bb.FirstInstruction(); !inst.IsNil(); inst = llvm.NextInstruction(inst) {
					if !inst.IsAStoreInst().IsNil() && inst.IsVolatile() && inst.Operand(1).Name() == "storage" {
						clears++
					}
				}
			}
			if clears != want {
				t.Errorf("%s: got %d lifetime clears, want %d", name, clears, want)
			}
		}
	})
}

func TestGCAllocationLayouts(t *testing.T) {
	t.Parallel()

	for _, passes := range []string{"", "function(memcpyopt)", "thinlto-pre-link<O2>", "thinlto-pre-link<Oz>"} {
		t.Run(passes, func(t *testing.T) {
			ctx := llvm.NewContext()
			defer ctx.Dispose()
			const path = "testdata/gc-stackslots.ll"
			ensureTestCacheFreshness(t, path)
			buf, err := llvm.NewMemoryBufferFromFile(path)
			if err != nil {
				t.Fatal(err)
			}
			mod, err := ctx.ParseIR(buf)
			if err != nil {
				t.Fatal(err)
			}
			defer mod.Dispose()

			ptrType := llvm.PointerType(ctx.Int8Type(), 0)
			roots := llvm.ConstArray(ptrType, []llvm.Value{mod.NamedGlobal("runtime.stackChainStart")})
			used := llvm.AddGlobal(mod, roots.Type(), "llvm.compiler.used")
			used.SetInitializer(roots)
			used.SetLinkage(llvm.AppendingLinkage)
			used.SetSection("llvm.metadata")
			keep := map[string]bool{
				"needsStackSlots":                    true,
				"deadByteArrayStackAllocation":       true,
				"deadPointerFreeStackAllocation":     true,
				"promotedPointerFreeStackAllocation": true,
				"promotedPointerStackAllocation":     true,
				"mergedPointerFreeStackAllocation":   true,
			}
			for fn := mod.FirstFunction(); !fn.IsNil(); {
				next := llvm.NextFunction(fn)
				if !fn.FirstBasicBlock().IsNil() && !keep[fn.Name()] {
					name := fn.Name()
					fn.SetName(name + ".defined")
					declaration := llvm.AddFunction(mod, name, fn.GlobalValueType())
					fn.ReplaceAllUsesWith(declaration)
					fn.EraseFromParentAsFunction()
				}
				fn = next
			}
			transform.OptimizeAllocs(mod, nil, 256, nil)
			if passes != "" {
				po := llvm.NewPassBuilderOptions()
				defer po.Dispose()
				if err := mod.RunPasses(passes, llvm.TargetMachine{}, po); err != nil {
					t.Fatal(err)
				}
			}
			if !transform.MakeGCStackSlots(mod) {
				t.Fatalf("GC pass did not run:\n%s", mod.String())
			}
			if err := llvm.VerifyModule(mod, llvm.ReturnStatusAction); err != nil {
				t.Fatal(err)
			}

			for name, wantClear := range map[string]bool{
				"deadByteArrayStackAllocation":       true,
				"deadPointerFreeStackAllocation":     false,
				"promotedPointerFreeStackAllocation": false,
				"promotedPointerStackAllocation":     true,
				"mergedPointerFreeStackAllocation":   true,
			} {
				fn := mod.NamedFunction(name)
				var allocas, marked, clears int
				for bb := fn.FirstBasicBlock(); !bb.IsNil(); bb = llvm.NextBasicBlock(bb) {
					for inst := bb.FirstInstruction(); !inst.IsNil(); inst = llvm.NextInstruction(inst) {
						if !inst.IsAAllocaInst().IsNil() && inst.Name() != "gc.stackobject" {
							allocas++
							if llvmutil.IsPointerFreeAlloca(inst) {
								marked++
							}
						}
						if !inst.IsAStoreInst().IsNil() && inst.IsVolatile() && inst.Operand(1).Name() != "gc.stackobject" {
							clears++
						}
					}
				}
				wantPointerFree := !wantClear || (name == "mergedPointerFreeStackAllocation" && allocas > 1)
				wantClear = wantClear && allocas != 0
				if name == "mergedPointerFreeStackAllocation" && passes == "function(memcpyopt)" && llvmutil.Version() >= 22 && allocas != 1 {
					t.Errorf("expected memcpy optimization to merge allocas:\n%s", fn.String())
				}
				if (clears != 0) != wantClear || (marked != 0) != wantPointerFree {
					t.Errorf("%s: got %d pointer-free allocas and %d clears, want clearing %t:\n%s", name, marked, clears, wantClear, fn.String())
				}
			}
		})
	}
}

func TestMakeGCGlobalRootsAVR(t *testing.T) {
	t.Parallel()
	testTransform(t, "testdata/gc-globals-avr", func(mod llvm.Module) {
		transform.MakeGCStackSlots(mod)
	})
}

// TestGCStackSlotCounts checks slot counts without comparing unrelated IR to a golden file.
// See https://github.com/tinygo-org/tinygo/pull/5762#discussion_r4113188988.
func TestGCStackSlotCounts(t *testing.T) {
	t.Parallel()

	want := map[string]int{
		// The inputs of the acyclic merge feeding this loop do not need slots.
		"acyclicPhiIntoLoop": 3,
		// Every input here does, and must keep them.
		"nestedLoopPhis": 4,
	}

	ctx := llvm.NewContext()
	defer ctx.Dispose()
	ensureTestCacheFreshness(t, "testdata/gc-slotcounts.ll")
	buf, err := llvm.NewMemoryBufferFromFile("testdata/gc-slotcounts.ll")
	if err != nil {
		t.Fatalf("could not read file: %v", err)
	}
	mod, err := ctx.ParseIR(buf)
	if err != nil {
		t.Fatalf("could not load module:\n%v", err)
	}
	defer mod.Dispose()

	transform.MakeGCStackSlots(mod)
	if err := llvm.VerifyModule(mod, llvm.PrintMessageAction); err != nil {
		t.Fatal("IR verification failed")
	}

	for name, want := range want {
		fn := mod.NamedFunction(name)
		if fn.IsNil() {
			t.Errorf("%s: not found in module", name)
			continue
		}
		got, err := stackSlotCount(fn)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("%s: got %d stack slots, want %d", name, got, want)
		}
	}
}

// stackSlotCount returns the number of pointer slots in the gc.stackobject of
// fn. The stack object is {parent, numSlots, slots...}, so the count is the
// number of struct fields beyond the first two.
func stackSlotCount(fn llvm.Value) (int, error) {
	entry := fn.EntryBasicBlock()
	for inst := entry.FirstInstruction(); !inst.IsNil(); inst = llvm.NextInstruction(inst) {
		if inst.IsAAllocaInst().IsNil() || inst.Name() != "gc.stackobject" {
			continue
		}
		return inst.AllocatedType().StructElementTypesCount() - 2, nil
	}
	return 0, errors.New("no gc.stackobject in entry block")
}
