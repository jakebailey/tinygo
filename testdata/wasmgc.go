package main

type wasmGCValue struct {
	value int
	next  *wasmGCValue
}

var wasmGCRoot = &wasmGCValue{value: 7}

func init() {
	println(wasmGCRoot.value)
}

func wasmGCProduce(ch chan int) {
	var head *wasmGCValue
	for i := 0; i < 10; i++ {
		head = &wasmGCValue{
			value: i,
			next:  head,
		}
	}

	total := 0
	for value := head; value != nil; value = value.next {
		total += value.value
	}
	ch <- total
}

func main() {
	ch := make(chan int)
	go wasmGCProduce(ch)
	println(<-ch)
	println(wasmGCRoot.value)

	left := 17
	right := 5
	minimum := -2147483647 - 1
	negativeOne := -1
	largeShift := uint(35)
	println(left/right, left%right, minimum/negativeOne)
	println(uint(left)/uint(right), uint(left)%uint(right))
	println(1<<largeShift, -8>>largeShift, uint(8)>>largeShift, 7&^3)
}
