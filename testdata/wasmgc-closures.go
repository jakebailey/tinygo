package main

type wasmGCClosureValue struct {
	value int
}

func main() {
	value := &wasmGCClosureValue{value: 40}
	add := func(delta int) int {
		return value.value + delta
	}
	println(add(2))
}
