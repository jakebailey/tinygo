package transform

import (
	"testing"

	"github.com/tinygo-org/tinygo/compileopts"
	"tinygo.org/x/go-llvm"
)

func TestPreserveCallerFrames(t *testing.T) {
	ctx := llvm.NewContext()
	defer ctx.Dispose()
	buf, err := llvm.NewMemoryBufferFromFile("testdata/callers.ll")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := ctx.ParseIR(buf)
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Dispose()

	config := &compileopts.Config{
		Options: &compileopts.Options{},
		Target: &compileopts.TargetSpec{
			GOOS:      "linux",
			GOARCH:    "amd64",
			Scheduler: "threads",
		},
	}
	preserveCallerFrames(mod, config)

	for _, name := range []string{
		"runtime.Callers",
		"directCaller",
		"callback",
		"indirectInvoker",
		"directParent",
		"testFunction",
	} {
		fn := mod.NamedFunction(name)
		attr := fn.GetStringAttributeAtIndex(-1, "frame-pointer")
		if attr.IsNil() || attr.GetStringValue() != "all" {
			t.Errorf("%s was not preserved", name)
		}
	}

	for _, name := range []string{
		"storeCallback",
		"storeInvoker",
		"secondLevelInvoker",
		"testHarness",
		"unrelated",
	} {
		fn := mod.NamedFunction(name)
		if attr := fn.GetStringAttributeAtIndex(-1, "frame-pointer"); !attr.IsNil() {
			t.Errorf("%s was preserved", name)
		}
	}

	alwaysInline := llvm.AttributeKindID("alwaysinline")
	if attr := mod.NamedFunction("callback").GetEnumAttributeAtIndex(-1, alwaysInline); !attr.IsNil() {
		t.Error("callback still has alwaysinline")
	}
}
