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
var wasmGCStructSliceGlobal = []wasmGCStructValue{{number: 6}}

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
	structSlice := make([]wasmGCStructValue, 2)
	println(structSlice[0].number, structSlice[0].pointer == nil)
	elementPointer := &structSlice[0]
	structSlice[0] = wasmGCStructValue{
		number:  7,
		pointer: &wasmGCStructNode{value: 8},
	}
	structSlice[1] = structSlice[0]
	structSlice[1].number++
	structSlice[1].pointer.value++
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
		elementPointer.number,
		structSlice[1].number,
		elementPointer.pointer.value,
		structSlice[1].pointer.value,
		wasmGCStructSliceGlobal[0].number,
	)
}
