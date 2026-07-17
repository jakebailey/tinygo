package main

type wasmGCStructNode struct {
	value int
}

type wasmGCStructValue struct {
	number  int
	pointer *wasmGCStructNode
}

type wasmGCConvertedStructValue struct {
	number  int
	pointer *wasmGCStructNode
}

var wasmGCStructGlobal = makeWasmGCStructValue()

func makeWasmGCStructValue() wasmGCStructValue {
	return wasmGCStructValue{
		number:  40,
		pointer: &wasmGCStructNode{value: 2},
	}
}

func replaceWasmGCStructFields(value wasmGCStructValue) wasmGCStructValue {
	value.number++
	value.pointer = &wasmGCStructNode{value: 3}
	return value
}

func mutateWasmGCStructTarget(value wasmGCStructValue) wasmGCStructValue {
	value.pointer.value++
	return value
}

func zeroWasmGCStructValue() wasmGCStructValue {
	var value wasmGCStructValue
	return value
}

func main() {
	original := makeWasmGCStructValue()
	replaced := replaceWasmGCStructFields(original)
	mutated := mutateWasmGCStructTarget(original)
	zero := zeroWasmGCStructValue()
	converted := wasmGCConvertedStructValue(replaced)
	globalSnapshot := wasmGCStructGlobal
	globalPointer := &wasmGCStructGlobal
	wasmGCStructGlobal = wasmGCStructValue{
		number:  50,
		pointer: &wasmGCStructNode{value: 4},
	}
	println(
		original.number,
		original.pointer.value,
		replaced.number,
		replaced.pointer.value,
		mutated.number,
		mutated.pointer.value,
		zero.number,
		zero.pointer == nil,
		converted.number,
		converted.pointer.value,
		globalSnapshot.number,
		globalSnapshot.pointer.value,
		globalPointer.number,
		globalPointer.pointer.value,
	)
}
