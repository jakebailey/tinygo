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

	done := make(chan int)
	other := &wasmGCClosureValue{value: 1}
	values := []int{1, 2}
	message := "spawned"
	for delta := 0; delta < 2; delta++ {
		go func(other *wasmGCClosureValue, values []int, message string, delta int) {
			done <- value.value + other.value + values[0] + len(message) + delta
		}(other, values, message, delta)
	}
	println(<-done + <-done)
}
