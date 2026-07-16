package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tinygo-org/tinygo/compileopts"
	"github.com/tinygo-org/tinygo/compiler/wasmgc"
	"github.com/tinygo-org/tinygo/goenv"
)

func buildWasmGC(result BuildResult, tmpdir string, config *compileopts.Config, program wasmgc.Program) (BuildResult, error) {
	wat, err := wasmgc.Compile(program)
	if err != nil {
		return result, err
	}

	watPath := filepath.Join(tmpdir, "main.wat")
	if err := os.WriteFile(watPath, []byte(wat), 0666); err != nil {
		return result, err
	}

	result.Executable = filepath.Join(tmpdir, "main.wasm")
	result.Binary = result.Executable
	optLevel, _, _ := config.OptLevel()
	args := []string{
		"-" + optLevel,
		"-g",
		"--enable-gc",
		"--enable-reference-types",
		"--enable-multivalue",
		watPath,
		"--output", result.Binary,
	}
	wasmopt := goenv.Get("WASMOPT")
	if config.Options.PrintCommands != nil {
		config.Options.PrintCommands(wasmopt, args...)
	}
	cmd := exec.Command(wasmopt, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return result, fmt.Errorf("wasm-opt failed: %w", err)
	}

	return result, nil
}
