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
	pointers := []*wasmGCClosureValue{{value: 3}}
	message := "spawned"
	for delta := 0; delta < 2; delta++ {
		go func(other *wasmGCClosureValue, values []int, pointers []*wasmGCClosureValue, message string, delta int) {
			done <- value.value + other.value + values[0] + pointers[0].value + len(message) + delta
		}(other, values, pointers, message, delta)
	}
	println(<-done + <-done)

	blocked := make(chan int, 1)
	blocked <- 0
	ready := make(chan int, 1)
	liveDone := make(chan int)
	livePointer := &wasmGCClosureValue{value: 5}
	liveValues := []int{6, 7}
	livePointers := []*wasmGCClosureValue{{value: 8}}
	liveMessage := "live"
	go func(pointer *wasmGCClosureValue, values []int, pointers []*wasmGCClosureValue, message string) {
		ready <- 1
		blocked <- 1
		liveDone <- pointer.value + values[0] + pointers[0].value + len(message)
	}(livePointer, liveValues, livePointers, liveMessage)
	println(<-ready)
	println(<-blocked)
	println(<-liveDone)
	println(value.value, other.value, values[1], pointers[0].value, len(message))
}
