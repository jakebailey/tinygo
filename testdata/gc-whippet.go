package main

import (
	"runtime"
	"unsafe"
)

type object struct {
	next *object
	data [64]byte
}

var interiorRoot *byte
var falseRoots []uintptr

func testGoroutineStack() {
	ready := make(chan struct{})
	resume := make(chan struct{})
	done := make(chan byte)
	go func() {
		value := new(object)
		value.data[0] = 21
		ready <- struct{}{}
		<-resume
		done <- value.data[0]
	}()
	<-ready
	runtime.GC()
	resume <- struct{}{}
	if <-done != 21 {
		panic("goroutine stack did not retain its allocation")
	}
}

//go:noinline
func leaveInteriorRoot() {
	value := new(object)
	value.data[len(value.data)-1] = 42
	interiorRoot = &value.data[len(value.data)-1]
}

//go:noinline
func clobberStack() {
	var words [4096]uintptr
	for i := range words {
		words[i] = uintptr(i)
	}
	runtime.KeepAlive(&words)
}

func main() {
	testGoroutineStack()

	leaveInteriorRoot()
	clobberStack()
	runtime.GC()
	if *interiorRoot != 42 {
		panic("interior pointer did not retain its allocation")
	}

	falseRoots = make([]uintptr, 8192)
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	for i := range falseRoots {
		value := new(object)
		falseRoots[i] = uintptr(unsafe.Pointer(value))
	}
	clobberStack()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if after.HeapInuse > baseline.HeapInuse+128*1024 {
		panic("pointer-free heap data retained objects")
	}
	println("ok")
}
