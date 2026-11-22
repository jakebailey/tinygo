package main

import (
	"slices"
	"strings"
	"testing"
)

func TestPackagePlan(t *testing.T) {
	var rules ruleSet
	rules.skipTest("math/rand", `^TestOne\d+$`)
	rules.skipTest("math/rand", `^TestTwo$/^Sub$`)
	rules.stackSizeMB("math/rand", 20)
	rules.short("math/rand")
	if rules.err != nil {
		t.Fatal(rules.err)
	}
	plan := rules.planPackage("math/rand")
	want := []string{"-short", "-stack-size=20MB", `-skip=^TestOne\d+$|^TestTwo$/^Sub$`}
	if got := plan.flags(); !slices.Equal(got, want) {
		t.Fatalf("flags = %v, want %v", got, want)
	}
}

func TestPackageExclusion(t *testing.T) {
	var rules ruleSet
	rules.skipPackage("crypto/internal/fips140/...")
	rules.skipPackage("crypto/subtle")
	for _, tc := range []struct {
		pkg     string
		skipped bool
	}{
		{"crypto/internal/fips140", true},
		{"crypto/internal/fips140/aes/gcm", true},
		{"crypto/internal/fips140deps", false},
		{"crypto/subtle", true},
		{"crypto/subtle/extra", false},
	} {
		plan := rules.planPackage(tc.pkg)
		if plan.skipped != tc.skipped {
			t.Errorf("%s: skipped = %t, want %t", tc.pkg, plan.skipped, tc.skipped)
		}
	}
}

func TestInvalidRules(t *testing.T) {
	for _, add := range []func(*ruleSet){
		func(r *ruleSet) { r.skipTest("math/rand", "[") },
		func(r *ruleSet) { r.stackSizeMB("math/rand", 0) },
		func(r *ruleSet) { r.skipPackage("crypto/.../aes") },
		func(r *ruleSet) { r.includePackage("./tests/...") },
	} {
		var rules ruleSet
		add(&rules)
		if _, err := planSuite(nil, rules, options{}); err == nil || !strings.Contains(err.Error(), "rules.go:") {
			t.Errorf("%v: expected rule validation error, got %v", rules, err)
		}
	}
	var rules ruleSet
	rules.stackSizeMB("math/rand", 1)
	rules.stackSizeMB("math/rand", 2)
	if _, err := planSuite([]string{"math/rand"}, rules, options{}); err == nil || !strings.Contains(err.Error(), "conflicting stack size") {
		t.Fatalf("expected conflicting stack sizes to fail, got %v", err)
	}
}

func TestSuiteSelection(t *testing.T) {
	packages := []string{"encoding/json", "sync/atomic", "compress/bzip2", "os"}
	linux := targetInfo{GOOS: "linux", GOARCH: "arm"}
	component := targetInfo{GOOS: "linux", GOARCH: "arm", BuildTags: []string{"tinygo.wasm", "tinygo.wasi", "wasip2"}}
	preview1 := targetInfo{GOOS: "wasip1", GOARCH: "wasm", BuildTags: []string{"tinygo.wasm", "tinygo.wasi"}}
	baremetal := targetInfo{GOOS: "linux", GOARCH: "arm", BuildTags: []string{"baremetal"}}
	browser := targetInfo{GOOS: "js", GOARCH: "wasm", BuildTags: []string{"tinygo.wasm"}}
	windows := targetInfo{GOOS: "windows", GOARCH: "amd64"}
	for _, tc := range []struct {
		name   string
		target targetInfo
		opts   options
		want   []string
	}{
		{"host full", linux, options{mode: modeFull}, []string{"compress/bzip2", "encoding/json -stack-size=20MB -skip=^TestHTTPDecoding$", "os", "sync/atomic"}},
		{"component fast", component, options{mode: modeFast}, []string{"./tests/runtime_wasi", "os"}},
		{"component slow", component, options{mode: modeSlow}, []string{"compress/bzip2"}},
		{"component bench", component, options{mode: modeFast, bench: true}, []string{"os"}},
		{"preview 1 full", preview1, options{mode: modeFull}, []string{"./tests/runtime_wasi", "compress/bzip2", "os"}},
		{"bare metal fast", baremetal, options{mode: modeFast}, nil},
		{"browser fast", browser, options{mode: modeFast}, nil},
		{"windows full", windows, options{mode: modeFull}, []string{"compress/bzip2", "encoding/json -run=^TestSynctestMarshal$", "os"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plans, err := planSuite(packages, rulesFor(tc.target, tc.opts, "linux"), tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, p := range plans {
				if !p.skipped {
					got = append(got, strings.Join(append([]string{p.name}, p.flags()...), " "))
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("selection = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWASIHostExclusion(t *testing.T) {
	target := targetInfo{GOOS: "wasip1", GOARCH: "wasm", BuildTags: []string{"tinygo.wasm", "tinygo.wasi"}}
	opts := options{mode: modeFull}
	for _, host := range []string{"linux", "windows"} {
		rules := rulesFor(target, opts, host)
		if rules.err != nil {
			t.Fatal(rules.err)
		}
		plan := rules.planPackage("html/template")
		want := []string{`^TestMaxExecDepth$`}
		if host == "windows" {
			want = append(want, `^TestEmptyTemplateHTML$`)
		}
		if !slices.Equal(plan.skips, want) {
			t.Errorf("%s host: skips = %v, want %v", host, plan.skips, want)
		}
	}
}
