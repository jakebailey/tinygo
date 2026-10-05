package interp

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"tinygo.org/x/go-llvm"
)

func TestInterp(t *testing.T) {
	for _, name := range []string{
		"basic",
		"phi",
		"consteval",
		"intrinsics",
		"copy",
		"interface",
		"revert",
		"store",
		"alloc",
		"slicedata",
		"aggregate",
		"fastrand",
		"ptrtoint",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runTest(t, "testdata/"+name)
		})
	}
}

func TestInterpFunctionUsageAttributes(t *testing.T) {
	for _, wholeProgram := range []bool{false, true} {
		name := "package"
		if wholeProgram {
			name = "program"
		}
		t.Run(name, func(t *testing.T) {
			ctx := llvm.NewContext()
			defer ctx.Dispose()
			mod := ctx.NewModule("test")
			defer mod.Dispose()
			mod.SetDataLayout("e-p:64:64-i64:64-n8:16:32:64")
			builder := ctx.NewBuilder()
			defer builder.Dispose()
			typ := llvm.FunctionType(ctx.VoidType(), nil, false)
			helper := llvm.AddFunction(mod, "helper", typ)
			helper.AddFunctionAttr(ctx.CreateStringAttribute("tinygo-reflect-method-names", "Alpha Beta"))
			helper.AddFunctionAttr(ctx.CreateStringAttribute("tinygo-reflect-structof", ""))
			builder.SetInsertPointAtEnd(ctx.AddBasicBlock(helper, "entry"))
			builder.CreateRetVoid()
			dead := llvm.AddFunction(mod, "dead", typ)
			dead.AddFunctionAttr(ctx.CreateStringAttribute("tinygo-reflect-method-names", "Unused"))
			builder.SetInsertPointAtEnd(ctx.AddBasicBlock(dead, "entry"))
			builder.CreateRetVoid()
			init := llvm.AddFunction(mod, "example.init", typ)
			init.AddFunctionAttr(ctx.CreateStringAttribute("tinygo-reflect-method", ""))
			init.AddFunctionAttr(ctx.CreateStringAttribute("tinygo-reflect-method-names", "Zeta Alpha"))
			init.AddFunctionAttr(ctx.CreateStringAttribute("tinygo-reflect-makefunc", ""))
			init.AddFunctionAttr(ctx.CreateStringAttribute("unrelated", ""))
			builder.SetInsertPointAtEnd(ctx.AddBasicBlock(init, "entry"))
			builder.CreateCall(typ, helper, nil, "")
			builder.CreateRetVoid()
			resultName := init.Name()
			if wholeProgram {
				initAll := llvm.AddFunction(mod, "runtime.initAll", typ)
				builder.SetInsertPointAtEnd(ctx.AddBasicBlock(initAll, "entry"))
				builder.CreateCall(typ, init, nil, "")
				builder.CreateRetVoid()
				resultName = initAll.Name()
				if err := Run(mod, time.Second, DefaultMaxInterpBlockEntries, false); err != nil {
					t.Fatal(err)
				}
			} else if err := RunFunc(init, time.Second, DefaultMaxInterpBlockEntries, false); err != nil {
				t.Fatal(err)
			}
			result := mod.NamedFunction(resultName)
			for kind, want := range map[string]string{
				"tinygo-reflect-method":       "",
				"tinygo-reflect-method-names": "Alpha Beta Zeta",
				"tinygo-reflect-makefunc":     "",
				"tinygo-reflect-structof":     "",
			} {
				attr := result.GetStringAttributeAtIndex(-1, kind)
				if attr.IsNil() {
					t.Errorf("lost %s", kind)
				} else if got := attr.GetStringValue(); got != want {
					t.Errorf("%s = %q, want %q", kind, got, want)
				}
			}
			if !result.GetStringAttributeAtIndex(-1, "unrelated").IsNil() {
				t.Error("copied an unrelated attribute")
			}
			if err := llvm.VerifyModule(mod, llvm.ReturnStatusAction); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func runTest(t *testing.T, pathPrefix string) {
	// Read the input IR.
	ctx := llvm.NewContext()
	defer ctx.Dispose()
	ensureTestCacheFreshness(t, pathPrefix+".ll")
	buf, err := llvm.NewMemoryBufferFromFile(pathPrefix + ".ll")
	if err != nil {
		t.Fatalf("could not read file %s: %v", pathPrefix+".ll", err)
	}
	mod, err := ctx.ParseIR(buf)
	if err != nil {
		t.Fatalf("could not load module:\n%v", err)
	}
	defer mod.Dispose()

	// Perform the transform.
	err = Run(mod, 10*time.Minute, DefaultMaxInterpBlockEntries, false)
	if err != nil {
		if err, match := err.(*Error); match {
			println(err.Error())
			if len(err.Inst) != 0 {
				println(err.Inst)
			}
			if len(err.Traceback) > 0 {
				println("\ntraceback:")
				for _, line := range err.Traceback {
					println(line.Pos.String() + ":")
					println(line.Inst)
				}
			}
		}
		t.Fatal(err)
	}

	// To be sure, verify that the module is still valid.
	if llvm.VerifyModule(mod, llvm.PrintMessageAction) != nil {
		t.FailNow()
	}

	// Run some cleanup passes to get easy-to-read outputs.
	to := llvm.NewPassBuilderOptions()
	defer to.Dispose()
	mod.RunPasses("globalopt,dse,adce", llvm.TargetMachine{}, to)

	// Read the expected output IR.
	out, err := os.ReadFile(pathPrefix + ".out.ll")
	if err != nil {
		t.Fatalf("could not read output file %s: %v", pathPrefix+".out.ll", err)
	}

	// See whether the transform output matches with the expected output IR.
	expected := string(out)
	actual := mod.String()
	if !fuzzyEqualIR(expected, actual) {
		t.Logf("output does not match expected output:\n%s", actual)
		t.Fail()
	}
}

// fuzzyEqualIR returns true if the two LLVM IR strings passed in are roughly
// equal. That means, only relevant lines are compared (excluding comments
// etc.).
func fuzzyEqualIR(s1, s2 string) bool {
	// Golden files are written using the pre-LLVM21 'nocapture' spelling,
	// which LLVM printed before any co-occurring attribute such as
	// 'readonly' (e.g. "ptr nocapture readonly"). LLVM 21+ prints the
	// equivalent 'captures(none)' instead, and after such attributes (e.g.
	// "ptr readonly captures(none)"). Normalize both name and position back
	// to the old spelling to keep a single golden file working across LLVM
	// versions.
	s1 = normalizeCapturesAttr(s1)
	s2 = normalizeCapturesAttr(s2)

	lines1 := filterIrrelevantIRLines(strings.Split(s1, "\n"))
	lines2 := filterIrrelevantIRLines(strings.Split(s2, "\n"))
	if len(lines1) != len(lines2) {
		return false
	}
	for i, line1 := range lines1 {
		line2 := lines2[i]
		if line1 != line2 {
			return false
		}
	}

	return true
}

// capturesNoneAttrRe matches a co-occurring attribute directly followed by
// 'captures(none)', which is how LLVM 21+ orders these two attributes when
// printing IR (the pre-LLVM21 'nocapture' attribute printed the other way
// around).
var capturesNoneAttrRe = regexp.MustCompile(`\b(readonly|readnone|writeonly|nonnull)\s+captures\(none\)`)

// normalizeCapturesAttr rewrites LLVM 21+'s 'captures(none)' attribute back
// to the pre-LLVM21 'nocapture' spelling and position, so golden IR files
// written against LLVM <21 keep matching.
func normalizeCapturesAttr(s string) string {
	s = capturesNoneAttrRe.ReplaceAllString(s, "nocapture $1")
	s = strings.ReplaceAll(s, "captures(none)", "nocapture")
	return s
}

// filterIrrelevantIRLines removes lines from the input slice of strings that
// are not relevant in comparing IR. For example, empty lines and comments are
// stripped out.
func filterIrrelevantIRLines(lines []string) []string {
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line) // drop '\r' on Windows
		if line == "" || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "source_filename = ") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// ensureTestCacheFreshness registers path as an input of the running test.
// see https://github.com/tinygo-org/tinygo/issues/5780.
func ensureTestCacheFreshness(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("could not stat test fixture %s: %v", path, err)
	}
}
