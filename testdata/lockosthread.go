package main

// Test LockOSThread with a scheduler that runs goroutines on a pool of threads.

import (
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
)

var initTid int

func init() {
	runtime.LockOSThread()
	initTid = syscall.Gettid()
}

func lockedWorker(wg *sync.WaitGroup, in, out chan int) {
	defer wg.Done()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tid := syscall.Gettid()
	for v := range in {
		if syscall.Gettid() != tid {
			panic("locked goroutine moved")
		}
		if v%10 == 0 {
			time.Sleep(time.Millisecond)
		}
		runtime.Gosched()
		out <- v
	}
}

func main() {
	if initTid != os.Getpid() {
		panic("init did not run on the main thread")
	}
	for i := 0; i < 100; i++ {
		runtime.Gosched()
		time.Sleep(10 * time.Microsecond)
		if syscall.Gettid() != os.Getpid() {
			panic("main goroutine left the main thread")
		}
	}
	println("main stays on the main thread")

	var wg sync.WaitGroup
	in, out := make(chan int), make(chan int)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go lockedWorker(&wg, in, out)
	}
	go func() {
		for i := 0; i < 1000; i++ {
			in <- i
		}
		close(in)
	}()
	sum := 0
	for i := 0; i < 1000; i++ {
		sum += <-out
	}
	wg.Wait()
	println("locked goroutines:", sum)

	// A goroutine that exits while locked takes its thread with it.
	tids := make(chan int)
	for i := 0; i < 20; i++ {
		go func() {
			runtime.LockOSThread()
			tids <- syscall.Gettid()
		}()
	}
	seen := map[int]bool{}
	for i := 0; i < 20; i++ {
		seen[<-tids] = true
	}
	println("exited locked threads were distinct:", len(seen) == 20)

	runtime.UnlockOSThread()
	done := make(chan bool)
	go func() {
		runtime.LockOSThread()
		runtime.UnlockOSThread()
		done <- true
	}()
	<-done
	println("ok")
}
