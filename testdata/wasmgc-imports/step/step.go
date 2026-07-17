package step

import "github.com/tinygo-org/tinygo/testdata/wasmgc-imports/state"

func init() {
	state.Add(1)
}
