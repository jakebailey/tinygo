package main

type wasmGCSliceValue struct {
	value int
}

var wasmGCSlicePointers = []*wasmGCSliceValue{{value: 5}}

func makeWasmGCValues(count int) []int {
	values := make([]int, count)
	for i := 0; i < len(values); i++ {
		values[i] = i * 3
	}
	return values
}

func sumWasmGCValues(values []int) int {
	total := 0
	for i := 0; i < len(values); i++ {
		total += values[i]
	}
	return total
}

func main() {
	window := makeWasmGCValues(8)[2:6]
	window[1]++
	println(sumWasmGCValues(window), len(window), cap(window))

	var values []int
	for i := 1; i <= 6; i++ {
		values = append(values, i)
	}
	println(sumWasmGCValues(values), len(values))

	appended := append(values[1:3], 99)
	println(sumWasmGCValues(appended), cap(appended), values[3])

	overlapped := append(values[:1], values...)
	println(sumWasmGCValues(overlapped), cap(overlapped))

	var bytes []byte
	bytes = append(bytes, 250, 6)
	bytes = append(bytes, "go"...)
	println(int(bytes[0]), int(bytes[1]), int(bytes[2]), int(bytes[3]))

	pointers := []*wasmGCSliceValue{{value: 10}, nil}
	println(pointers[1] == nil)
	pointers[1] = &wasmGCSliceValue{value: 20}
	pointers = append(pointers, &wasmGCSliceValue{value: 12})
	pointers = append(pointers[:1], pointers...)
	total := 0
	for _, pointer := range pointers {
		total += pointer.value
	}
	println(total, len(pointers), wasmGCSlicePointers[0].value)

	copySource := []int{1, 2, 3, 4}
	copyDestination := make([]int, 3)
	println(copy(copyDestination, copySource[1:]), sumWasmGCValues(copyDestination))
	copyRight := []int{1, 2, 3, 4}
	println(copy(copyRight[1:], copyRight[:3]), copyRight[0], copyRight[1], copyRight[2], copyRight[3])
	copyLeft := []int{1, 2, 3, 4}
	println(copy(copyLeft[:3], copyLeft[1:]), copyLeft[0], copyLeft[1], copyLeft[2], copyLeft[3])
	copyBytes := make([]byte, 3)
	println(copy(copyBytes, "wasm"), int(copyBytes[0]), int(copyBytes[1]), int(copyBytes[2]))
	pointerCopySource := []*wasmGCSliceValue{{value: 10}}
	pointerCopyDestination := make([]*wasmGCSliceValue, 1)
	pointerCopySlot := &pointerCopyDestination[0]
	println(copy(pointerCopyDestination, pointerCopySource), (*pointerCopySlot).value)
	*pointerCopySlot = &wasmGCSliceValue{value: 30}
	println(pointerCopySource[0].value, pointerCopyDestination[0].value)

	clearNumbers := []int{1, 2, 3}
	clear(clearNumbers[1:])
	println(clearNumbers[0], clearNumbers[1], clearNumbers[2])
	clearPointers := []*wasmGCSliceValue{{value: 4}, {value: 5}}
	clearPointerSlot := &clearPointers[0]
	clear(clearPointers[:1])
	println(*clearPointerSlot == nil, clearPointers[1].value)
	var nilNumbers []int
	clear(nilNumbers)
}
