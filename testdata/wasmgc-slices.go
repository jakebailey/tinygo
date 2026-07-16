package main

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
}
