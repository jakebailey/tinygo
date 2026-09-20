package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tinygo-org/tinygo/compileopts"
	"tinygo.org/x/go-llvm"
)

// Test whether the Clang generated "target-cpu" and "target-features"
// attributes match the CPU and Features property in TinyGo target files.
func TestClangAttributes(t *testing.T) {
	var targetNames = []string{
		// Please keep this list sorted!
		"atmega328p",
		"atmega1280",
		"atmega1284p",
		"atmega2560",
		"attiny85",
		"cortex-m0",
		"cortex-m0plus",
		"cortex-m3",
		"cortex-m33",
		"cortex-m4",
		"cortex-m7",
		"esp32c3",
		"esp32c6",
		"esp32h2",
		"esp32s3",
		"fe310",
		"gameboy-advance",
		"k210",
		"nintendoswitch",
		"riscv-qemu",
		"tkey",
		"uefi-amd64",
		"wasip1",
		"wasip2",
		"wasm",
		"wasm-unknown",
	}
	if hasBuiltinTools {
		// hasBuiltinTools is set when TinyGo is statically linked with LLVM,
		// which also implies it was built with Xtensa support.
		targetNames = append(targetNames, "esp32", "esp8266")
	}
	for _, targetName := range targetNames {
		t.Run(targetName, func(t *testing.T) {
			testClangAttributes(t, &compileopts.Options{Target: targetName})
		})
	}

	for _, options := range []*compileopts.Options{
		{GOOS: "linux", GOARCH: "386"},
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "linux", GOARCH: "arm", GOARM: "5,softfloat"},
		{GOOS: "linux", GOARCH: "arm", GOARM: "6,softfloat"},
		{GOOS: "linux", GOARCH: "arm", GOARM: "7,softfloat"},
		{GOOS: "linux", GOARCH: "arm", GOARM: "5,hardfloat"},
		{GOOS: "linux", GOARCH: "arm", GOARM: "6,hardfloat"},
		{GOOS: "linux", GOARCH: "arm", GOARM: "7,hardfloat"},
		{GOOS: "linux", GOARCH: "arm64"},
		{GOOS: "linux", GOARCH: "mips", GOMIPS: "hardfloat"},
		{GOOS: "linux", GOARCH: "mipsle", GOMIPS: "hardfloat"},
		{GOOS: "linux", GOARCH: "mips", GOMIPS: "softfloat"},
		{GOOS: "linux", GOARCH: "mipsle", GOMIPS: "softfloat"},
		{GOOS: "darwin", GOARCH: "amd64"},
		{GOOS: "darwin", GOARCH: "arm64"},
		{GOOS: "windows", GOARCH: "386"},
		{GOOS: "windows", GOARCH: "amd64"},
		{GOOS: "windows", GOARCH: "arm64"},
	} {
		name := "GOOS=" + options.GOOS + ",GOARCH=" + options.GOARCH
		if options.GOARCH == "arm" {
			name += ",GOARM=" + options.GOARM
		}
		if options.GOARCH == "mips" || options.GOARCH == "mipsle" {
			name += ",GOMIPS=" + options.GOMIPS
		}
		t.Run(name, func(t *testing.T) {
			testClangAttributes(t, options)
		})
	}
}

func testClangAttributes(t *testing.T, options *compileopts.Options) {
	testDir := t.TempDir()

	ctx := llvm.NewContext()
	defer ctx.Dispose()

	target, err := compileopts.LoadTarget(options)
	if err != nil {
		t.Fatalf("could not load target: %s", err)
	}
	config := compileopts.Config{
		Options: options,
		Target:  target,
	}

	// Create a very simple C input file.
	srcpath := filepath.Join(testDir, "test.c")
	err = os.WriteFile(srcpath, []byte("int add(int a, int b) { return a + b; }"), 0o666)
	if err != nil {
		t.Fatalf("could not write target file %s: %s", srcpath, err)
	}

	// Compile this file using Clang.
	outpath := filepath.Join(testDir, "test.bc")
	flags := append([]string{"-c", "-emit-llvm", "-o", outpath, srcpath}, config.CFlags(false)...)
	if config.GOOS() == "darwin" {
		// Silence some warnings that happen when testing GOOS=darwin on
		// something other than MacOS.
		flags = append(flags, "-Wno-missing-sysroot", "-Wno-incompatible-sysroot")
	}
	err = runCCompiler(flags...)
	if err != nil {
		t.Fatalf("failed to compile %s: %s", srcpath, err)
	}

	// Read the resulting LLVM bitcode.
	mod, err := ctx.ParseBitcodeFile(outpath)
	if err != nil {
		t.Fatalf("could not parse bitcode file %s: %s", outpath, err)
	}
	defer mod.Dispose()

	// Check whether the LLVM target matches.
	// Use ClangTriple since LLVM 22 normalizes wasm32-unknown-wasi to wasip1.
	expectedTriple := compileopts.ClangTriple(config.Triple())
	if mod.Target() != expectedTriple {
		t.Errorf("target has LLVM triple %#v but Clang makes it LLVM triple %#v", expectedTriple, mod.Target())
	}

	// Check the "target-cpu" and "target-features" string attribute of the add
	// function.
	add := mod.NamedFunction("add")
	var cpu, features string
	cpuAttr := add.GetStringAttributeAtIndex(-1, "target-cpu")
	featuresAttr := add.GetStringAttributeAtIndex(-1, "target-features")
	if !cpuAttr.IsNil() {
		cpu = cpuAttr.GetStringValue()
	}
	if !featuresAttr.IsNil() {
		features = featuresAttr.GetStringValue()
	}
	if cpu != config.CPU() {
		t.Errorf("target has CPU %#v but Clang makes it CPU %#v", config.CPU(), cpu)
	}
	if features != config.Features() {
		if hasBuiltinTools || runtime.GOOS != "linux" {
			// Skip this step when using an external Clang invocation on Linux.
			// The reason is that Debian has patched Clang in a way that
			// modifies the LLVM features string, changing lots of FPU/float
			// related flags. We want to test vanilla Clang, not Debian Clang.
			//
			// Rather than requiring an exact match (which breaks across LLVM
			// versions as new features are added), check that every feature
			// TinyGo specifies is consistent with what Clang produces:
			//  - A "+feature" in TinyGo must appear in Clang's output.
			//  - A "-feature" in TinyGo must not be "+feature" in Clang's output.
			checkFeatureFlags(t, config.Features(), features)
		}
	}
}

// checkFeatureFlags verifies that all features specified by TinyGo's target
// configuration are consistent with Clang's output. This allows Clang to add
// new features across LLVM versions without breaking the test.
func checkFeatureFlags(t *testing.T, targetFeatures, clangFeatures string) {
	t.Helper()

	// Build a set of Clang's features for fast lookup.
	clangSet := make(map[string]bool) // feature name -> enabled
	for _, f := range strings.Split(clangFeatures, ",") {
		f = strings.TrimSpace(f)
		if len(f) < 2 {
			continue
		}
		enabled := f[0] == '+'
		name := f[1:]
		clangSet[name] = enabled
	}

	// Check each feature that TinyGo specifies.
	var missing, conflicts []string
	for _, f := range strings.Split(targetFeatures, ",") {
		f = strings.TrimSpace(f)
		if len(f) < 2 {
			continue
		}
		wantEnabled := f[0] == '+'
		name := f[1:]

		clangEnabled, inClang := clangSet[name]
		if wantEnabled && (!inClang || !clangEnabled) {
			// TinyGo requires +feature but Clang doesn't enable it.
			missing = append(missing, f)
		} else if !wantEnabled && inClang && clangEnabled {
			// TinyGo requires -feature but Clang enables it.
			conflicts = append(conflicts, fmt.Sprintf("target has %q but Clang has %q", f, "+"+name))
		}
	}

	if len(missing) > 0 {
		t.Errorf("target specifies features not present in Clang output: %s\n\ttarget features: %s\n\tclang features:  %s",
			strings.Join(missing, ", "), targetFeatures, clangFeatures)
	}
	if len(conflicts) > 0 {
		t.Errorf("target disables features that Clang enables: %s",
			strings.Join(conflicts, "; "))
	}
}

func TestLinkReflectAttributes(t *testing.T) {
	kinds := []string{
		"tinygo-reflect-method",
		"tinygo-reflect-method-names",
		"tinygo-reflect-makefunc",
		"tinygo-reflect-structof",
	}
	for _, declarationFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("declaration-first=%v", declarationFirst), func(t *testing.T) {
			ctx := llvm.NewContext()
			defer ctx.Dispose()
			mod := ctx.NewModule("destination")
			defer mod.Dispose()
			pkgMod := ctx.NewModule("source")
			builder := ctx.NewBuilder()
			defer builder.Dispose()
			typ := llvm.FunctionType(ctx.VoidType(), nil, false)
			dst := llvm.AddFunction(mod, "lookup", typ)
			src := llvm.AddFunction(pkgMod, "lookup", typ)
			declaration, definition := dst, src
			if !declarationFirst {
				declaration, definition = src, dst
			}
			builder.SetInsertPointAtEnd(ctx.AddBasicBlock(definition, "entry"))
			builder.CreateRetVoid()
			for _, kind := range kinds {
				value := ""
				if kind == "tinygo-reflect-method-names" {
					value = "Keep"
					definition.AddFunctionAttr(ctx.CreateStringAttribute(kind, "Other Keep"))
				}
				declaration.AddFunctionAttr(ctx.CreateStringAttribute(kind, value))
			}
			if err := linkPackageModule(mod, pkgMod); err != nil {
				t.Fatal(err)
			}
			for _, kind := range kinds {
				attr := mod.NamedFunction("lookup").GetStringAttributeAtIndex(-1, kind)
				if attr.IsNil() {
					t.Errorf("lost %s", kind)
					continue
				}
				want := ""
				if kind == "tinygo-reflect-method-names" {
					want = "Keep Other"
				}
				if got := attr.GetStringValue(); got != want {
					t.Errorf("%s = %q, want %q", kind, got, want)
				}
			}
		})
	}
}

// This TestMain is necessary because TinyGo may also be invoked to run certain
// LLVM tools in a separate process. Not capturing these invocations would lead
// to recursive tests.
func TestMain(m *testing.M) {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "clang", "ld.lld", "wasm-ld":
			// Invoke a specific tool.
			err := RunTool(os.Args[1], os.Args[2:]...)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	// Run normal tests.
	os.Exit(m.Run())
}

func TestLibrarySourcePatch(t *testing.T) {
	patch := "--- a/source.c\n+++ b/source.c\n@@ -1,3 +1,4 @@\n first\n-old\n+new\n+extra\n last\n"
	for _, tc := range []struct {
		name  string
		patch string
		want  string
	}{
		{"replace", patch, "first\nnew\nextra\nlast\n"},
		{"insert", "--- a/source.c\n+++ b/source.c\n@@ -0,0 +1 @@\n+start\n", "start\nfirst\nold\nlast\n"},
		{"delete", "--- a/source.c\n+++ b/source.c\n@@ -2 +1,0 @@\n-old\n", "first\nlast\n"},
		{"multiple", "--- a/source.c\n+++ b/source.c\n@@ -1 +1 @@\n-first\n+begin\n@@ -3 +3 @@\n-last\n+end\n", "begin\nold\nend\n"},
		{"context", strings.Replace(patch, "-old", "-wrong", 1), ""},
		{"path", strings.ReplaceAll(patch, "source.c", "../outside.c"), ""},
		{"absolute", strings.ReplaceAll(patch, "source.c", "/outside.c"), ""},
		{"rename", strings.Replace(patch, "+++ b/source.c", "+++ b/other.c", 1), ""},
		{"count", strings.Replace(patch, "-1,3 +1,4", "-1,2 +1,4", 1), ""},
		{"position", strings.Replace(patch, "-1,3 +1,4", "-1,3 +2,4", 1), ""},
		{"truncated", strings.TrimSuffix(patch, " last\n"), ""},
		{"no-files", "diff --git a/source.c b/source.c\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "source.c")
			if err := os.WriteFile(path, []byte("first\nold\nlast\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			err := applyLibraryPatch(dir, []byte(tc.patch))
			if tc.want == "" {
				if err == nil {
					t.Fatal("invalid patch accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("private-copy", func(t *testing.T) {
		source := t.TempDir()
		if err := os.Mkdir(filepath.Join(source, "include"), 0o755); err != nil {
			t.Fatal(err)
		}
		for name, contents := range map[string]string{
			"source.c":         "first\nold\nlast\n",
			"include/header.h": "original header\n",
			".git":             "not source\n",
		} {
			if err := os.WriteFile(filepath.Join(source, name), []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chmod(filepath.Join(source, "include/header.h"), 0o444); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(t.TempDir(), "sources")
		fullPatch := patch + "--- a/include/header.h\n+++ b/include/header.h\n@@ -1 +1 @@\n-original header\n+patched header\n"
		if err := prepareLibrarySources(source, destination, []byte(fullPatch)); err != nil {
			t.Fatal(err)
		}
		for path, want := range map[string]string{
			filepath.Join(source, "source.c"):              "first\nold\nlast\n",
			filepath.Join(source, "include/header.h"):      "original header\n",
			filepath.Join(destination, "source.c"):         "first\nnew\nextra\nlast\n",
			filepath.Join(destination, "include/header.h"): "patched header\n",
		} {
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Fatalf("%s: got %q, want %q", path, got, want)
			}
		}
		if _, err := os.Stat(filepath.Join(destination, ".git")); !os.IsNotExist(err) {
			t.Fatalf("copied repository metadata: %v", err)
		}
	})
}

func TestLibraryPatchCacheKey(t *testing.T) {
	config := &compileopts.Config{
		Options: &compileopts.Options{},
		Target:  &compileopts.TargetSpec{Triple: "x86_64-unknown-linux-musl", Libc: "musl"},
	}
	library := Library{name: "bdwgc"}
	original := library.cachePath(config)
	if original != config.LibraryPath("bdwgc") {
		t.Fatal("unpatched library cache changed")
	}
	library.sourcePatch = []byte("first patch")
	first := library.cachePath(config)
	if first == original || first != library.cachePath(config) {
		t.Fatal("patch cache key is missing or unstable")
	}
	library.sourcePatch = []byte("second patch")
	if library.cachePath(config) == first {
		t.Fatal("patch change did not invalidate the archive")
	}
}

func TestBoehmSourcePatch(t *testing.T) {
	source := BoehmGC.sourceDir()
	path := filepath.Join(source, "include/private/gc_priv.h")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "bdwgc")
	if err := prepareLibrarySources(source, destination, BoehmGC.sourcePatch); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("collector source changed")
	}
	patched, err := os.ReadFile(filepath.Join(destination, "include/private/gc_priv.h"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patched), "hb_alloc_bits") {
		t.Fatal("allocation bitmap header was not patched")
	}
	if err := applyLibraryPatch(destination, BoehmGC.sourcePatch); err == nil {
		t.Fatal("patch applied twice instead of rejecting mismatched sources")
	}
}
