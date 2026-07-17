package helper

import (
	"github.com/tinygo-org/tinygo/testdata/wasmgc-imports/state"
	_ "github.com/tinygo-org/tinygo/testdata/wasmgc-imports/step"
)

func init() {
	state.Add(1)
}

type Value struct {
	Number int
}

func NewValue() *Value {
	return &Value{Number: state.Value()}
}

func Message() string {
	return "imported"
}
