package main

type wasmGCValue struct {
	value int
}

func wasmGCProduce(ch chan int) {
	value := &wasmGCValue{value: 42}
	ch <- value.value
}

func main() {
	ch := make(chan int)
	go wasmGCProduce(ch)
	println(<-ch)
}
