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
	grownStructSlice := append(structSlice, structSlice[0])
	grownStructSlice[0].number++
	grownStructSlice[2].number += 2
	grownStructSlice[2].pointer.value++
	reusedStructSlice := make([]wasmGCStructValue, 1, 3)
	reusedStructSlice[0].number = 5
	extendedStructSlice := reusedStructSlice[:3]
	spareStructPointer := &extendedStructSlice[1]
	reusedStructSlice = append(reusedStructSlice, reusedStructSlice[0], reusedStructSlice[0])
	reusedStructSlice[1].number++
	overlappingStructSlice := make([]wasmGCStructValue, 2, 4)
	overlappingStructSlice[0].number = 1
	overlappingStructSlice[1].number = 2
	overlappingStructSlice = append(overlappingStructSlice[:1], overlappingStructSlice...)
	overlappingStructSlice[2].number++
	zeroDestinationSlice := make([]wasmGCStructValue, 1, 2)
	zeroDestinationExtended := zeroDestinationSlice[:2]
	zeroDestinationPointer := &zeroDestinationExtended[1]
	zeroDestinationPointer.number = 99
	zeroSourceSlice := make([]wasmGCStructValue, 1)
	zeroDestinationSlice = append(zeroDestinationSlice, zeroSourceSlice...)
	emptyStructSlice := append(make([]struct{}, 0, 1), struct{}{})
	copySourceSlice := []wasmGCStructValue{
		{number: 1, pointer: &wasmGCStructNode{value: 4}},
		{number: 2},
	}
	copyDestinationSlice := make([]wasmGCStructValue, 2)
	copyDestinationPointer := &copyDestinationSlice[0]
	copyDestinationPointer.number = 99
	copyCount := copy(copyDestinationSlice, copySourceSlice)
	copyDestinationSlice[0].number++
	copyDestinationSlice[0].pointer.value++
	copyDestinationSlice[1].number++
	backwardCopySlice := []wasmGCStructValue{{number: 1}, {number: 2}, {number: 3}}
	backwardCopyPointer := &backwardCopySlice[1]
	backwardCopyCount := copy(backwardCopySlice[1:], backwardCopySlice[:2])
	forwardCopySlice := []wasmGCStructValue{{number: 1}, {number: 2}, {number: 3}}
	forwardCopyPointer := &forwardCopySlice[0]
	forwardCopyCount := copy(forwardCopySlice[:2], forwardCopySlice[1:])
	zeroCopyDestination := []wasmGCStructValue{{number: 99, pointer: &wasmGCStructNode{value: 5}}}
	zeroCopyPointer := &zeroCopyDestination[0]
	zeroCopyCount := copy(zeroCopyDestination, make([]wasmGCStructValue, 1))
	emptyCopyCount := copy(make([]struct{}, 1), make([]struct{}, 2))
	var nilCopySlice []wasmGCStructValue
	nilCopyCount := copy(nilCopySlice, copySourceSlice)
	equalityNode := &wasmGCStructNode{value: 7}
	equalityLeft := wasmGCStructValue{number: 3, pointer: equalityNode}
	equalityRight := wasmGCStructValue{number: 3, pointer: equalityNode}
	equalityDifferentNumber := wasmGCStructValue{number: 4, pointer: equalityNode}
	equalityDifferentPointer := wasmGCStructValue{number: 3, pointer: &wasmGCStructNode{value: 7}}
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
		elementPointer.number,
		grownStructSlice[0].number,
		grownStructSlice[2].number,
		elementPointer.pointer.value,
		spareStructPointer.number,
		len(reusedStructSlice),
		overlappingStructSlice[1].number,
		overlappingStructSlice[2].number,
		zeroDestinationPointer.number,
		zeroDestinationPointer.pointer == nil,
		len(emptyStructSlice),
		cap(emptyStructSlice),
		copyCount,
		copyDestinationPointer.number,
		copySourceSlice[0].number,
		copySourceSlice[0].pointer.value,
		copyDestinationSlice[1].number,
		copySourceSlice[1].number,
		backwardCopyCount,
		backwardCopySlice[0].number,
		backwardCopyPointer.number,
		backwardCopySlice[2].number,
		forwardCopyCount,
		forwardCopyPointer.number,
		forwardCopySlice[1].number,
		forwardCopySlice[2].number,
		zeroCopyCount,
		zeroCopyPointer.number,
		zeroCopyPointer.pointer == nil,
		emptyCopyCount,
		nilCopyCount,
		equalityLeft == equalityRight,
		equalityLeft != equalityDifferentNumber,
		equalityLeft == equalityDifferentPointer,
		wasmGCStructValue{} == wasmGCStructValue{},
		struct{}{} == struct{}{},
	)
}
