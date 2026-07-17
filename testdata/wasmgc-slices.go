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
}
