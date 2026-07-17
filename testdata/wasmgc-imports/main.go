package main

import "github.com/tinygo-org/tinygo/testdata/wasmgc-imports/helper"

func main() {
	value := helper.NewValue()
	message := helper.Message()
	println(value.Number, len(message), message[0])
}
