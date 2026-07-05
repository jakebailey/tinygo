package builder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinygo-org/tinygo/compileopts"
	"github.com/tinygo-org/tinygo/goenv"
)

func TestLibraryTrimPathCache(t *testing.T) {
	t.Setenv("GOCACHEPROG", "")
	const triple = "x86_64-unknown-linux-gnu"
	for _, tc := range []struct {
		name       string
		trimPath   bool
		cachedName string
		wantName   string
		rebuild    bool
	}{
		{"trimmed-old-cache", true, "probe+trimpath-v1-" + triple, "probe+trimpath-v2-" + triple, true},
		{"trimmed-current-cache", true, "probe+trimpath-v2-" + triple, "probe+trimpath-v2-" + triple, false},
		{"untrimmed-cache", false, "probe-" + triple, "probe-" + triple, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CACHE_HOME", home)
			t.Setenv("LocalAppData", home)
			cache, err := newBuildCache(goenv.Get("GOCACHE"))
			if err != nil {
				t.Fatal(err)
			}
			defer cache.Close()
			sourceDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(sourceDir, "probe.c"), []byte("int probe(void) { return 1; }\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			lib := &Library{
				name:           "probe",
				cflags:         func(string, string) []string { return nil },
				sourceDir:      func() string { return sourceDir },
				librarySources: func(string, bool) ([]string, error) { return []string{"probe.c"}, nil },
			}
			config := &compileopts.Config{
				Options: &compileopts.Options{TrimPath: tc.trimPath},
				Target:  &compileopts.TargetSpec{Triple: triple, GOOS: "linux", GOARCH: "amd64"},
			}
			input, err := lib.cacheInput(config)
			if err != nil {
				t.Fatal(err)
			}
			config.LibraryKeys = map[string]string{lib.name: input.key()}
			suffix := "-h" + input.key()
			cachedArchive := cache.filePath("lib", tc.cachedName+suffix)
			if err := os.MkdirAll(filepath.Dir(cachedArchive), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cachedArchive, []byte("cached library"), 0o644); err != nil {
				t.Fatal(err)
			}
			jobs, abort, err := lib.load(cache, config, t.TempDir(), input)
			if err != nil {
				t.Fatal(err)
			}
			defer abort()
			if len(jobs) != 1 {
				t.Fatalf("library jobs = %d, want 1", len(jobs))
			}
			job := jobs[0]
			want := cache.filePath("lib", tc.wantName+suffix)
			if got := len(job.dependencies) != 0; got != tc.rebuild {
				t.Fatalf("library rebuild = %t, want %t", got, tc.rebuild)
			}
			if err := runJobs(job, make(chan struct{}, 1)); err != nil {
				t.Fatal(err)
			}
			if job.result != want {
				t.Fatalf("library path = %q, want %q", job.result, want)
			}
			if tc.rebuild {
				if _, err := os.Stat(want); err != nil {
					t.Fatal(err)
				}
				cachedJobs, abort, err := lib.load(cache, config, t.TempDir(), input)
				if err != nil {
					t.Fatal(err)
				}
				defer abort()
				if len(cachedJobs) != 1 {
					t.Fatalf("cached library jobs = %d, want 1", len(cachedJobs))
				}
				cached := cachedJobs[0]
				if cached.result != want || len(cached.dependencies) != 0 {
					t.Fatalf("rebuilt library was not cached: %q", cached.result)
				}
			}
		})
	}
}
