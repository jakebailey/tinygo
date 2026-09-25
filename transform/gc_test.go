package transform_test

import (
	"testing"

	"github.com/tinygo-org/tinygo/transform"
	"tinygo.org/x/go-llvm"
)

func TestMakeGCStackSlots(t *testing.T) {
	t.Parallel()
	testTransform(t, "testdata/gc-stackslots", func(mod llvm.Module) {
		transform.MakeGCStackSlots(mod)
	})
}

func TestGCRootCallOptimization(t *testing.T) {
	t.Parallel()
	testTransform(t, "testdata/gc-root-call", func(mod llvm.Module) {
		po := llvm.NewPassBuilderOptions()
		defer po.Dispose()
		if err := mod.RunPasses("thinlto-pre-link<Oz>", llvm.TargetMachine{}, po); err != nil {
			t.Fatal(err)
		}
	})
}

func TestMakeGCGlobalRootsAVR(t *testing.T) {
	t.Parallel()
	testTransform(t, "testdata/gc-globals-avr", func(mod llvm.Module) {
		transform.MakeGCStackSlots(mod)
	})
}
