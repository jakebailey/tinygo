package main

import (
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
)

var retiredTask unsafe.Pointer
var finalized atomic.Int32

func main() {
	done := make(chan struct{})
	go func() {
		retiredTask = currentTask()
		p := new([128]byte)
		runtime.SetFinalizer(p, func(*[128]byte) { finalized.Add(1) })
		runtime.KeepAlive(p)
		close(done)
	}()
	<-done
	if retiredTask == nil {
		panic("missing retired task")
	}
	for i := 0; i < 100 && finalized.Load() == 0; i++ {
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	if finalized.Load() != 1 {
		panic("retired task retained its stack roots")
	}
	runtime.KeepAlive(retiredTask)
	println("ok")
}
