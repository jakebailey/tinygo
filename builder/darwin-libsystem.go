package builder

import (
	"path/filepath"
	"strings"

	"github.com/tinygo-org/tinygo/compileopts"
	"github.com/tinygo-org/tinygo/goenv"
)

// Create a job that builds a Darwin libSystem.dylib stub library. This library
// contains all the symbols needed so that we can link against it, but it
// doesn't contain any real symbol implementations.
func makeDarwinLibSystemJob(config *compileopts.Config, tmpdir string) *compileJob {
	return &compileJob{
		description: "compile Darwin libSystem.dylib",
		run: func(job *compileJob) error {
			arch, _, _ := strings.Cut(config.Triple(), "-")
			job.result = filepath.Join(tmpdir, "libSystem.dylib")
			root := goenv.Get("TINYGOROOT")
			sources := []string{
				filepath.Join(root, "lib/macos-minimal-sdk/src", arch, "libSystem.s"),
				filepath.Join(root, "src/runtime/libsystem_spawn.S"),
			}

			var objects []string
			for _, source := range sources {
				obj := filepath.Join(tmpdir, filepath.Base(source)+".o")
				flags := []string{"-nostdlib", "--target=" + config.Triple(), "-c", "-o", obj, source}
				if config.Options.PrintCommands != nil {
					config.Options.PrintCommands("clang", flags...)
				}
				if err := runCCompiler(flags...); err != nil {
					return err
				}
				objects = append(objects, obj)
			}

			// Link object file to dynamic library.
			platformVersion := strings.TrimPrefix(strings.Split(config.Triple(), "-")[2], "macosx")
			flags := []string{
				"-flavor", "darwin",
				"-demangle",
				"-dynamic",
				"-dylib",
				"-arch", arch,
				"-platform_version", "macos", platformVersion, platformVersion,
				"-install_name", "/usr/lib/libSystem.B.dylib",
				"-o", job.result,
			}
			flags = append(flags, objects...)
			if config.Options.PrintCommands != nil {
				config.Options.PrintCommands("ld.lld", flags...)
			}
			return link("ld.lld", flags)
		},
	}
}
