package main

// Tests for schedulers that run goroutines on multiple threads. The output
// must not depend on GOMAXPROCS.

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	burst()
	blockingCalls()
	parallelGC()
	timers()
	shutdown()
}

// Start many goroutines at once.
func burst() {
	const n = 2000
	results := make(chan int, 100)
	for i := 0; i < n; i++ {
		go func(i int) {
			results <- i
		}(i)
	}
	sum := 0
	for i := 0; i < n; i++ {
		sum += <-results
	}
	println("burst:", sum)
}

// Block more goroutines in system calls than GOMAXPROCS allows to run. The
// goroutine that unblocks them must still run.
func blockingCalls() {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	n := runtime.GOMAXPROCS(0) + 2
	var wg sync.WaitGroup
	var total atomic.Int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var buf [1]byte
			if _, err := r.Read(buf[:]); err != nil {
				panic(err)
			}
			total.Add(int32(buf[0]))
		}()
	}
	time.Sleep(10 * time.Millisecond)
	go func() {
		for i := 0; i < n; i++ {
			w.Write([]byte{1})
		}
	}()
	wg.Wait()
	println("blocking calls:", total.Load() == int32(n))
	r.Close()
	w.Close()
}

type node struct {
	next  *node
	value int
}

// Allocate and collect garbage from many goroutines at the same time.
func parallelGC() {
	var wg sync.WaitGroup
	var ok atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for round := 0; round < 20; round++ {
				var head *node
				for j := 0; j < 1000; j++ {
					head = &node{next: head, value: j}
				}
				if i == 0 && round%5 == 0 {
					runtime.GC()
				}
				sum := 0
				for p := head; p != nil; p = p.next {
					sum += p.value
				}
				if sum != 999*1000/2 {
					panic("corrupted list")
				}
			}
			ok.Add(1)
		}(i)
	}
	wg.Wait()
	println("parallel GC:", ok.Load())
}

// Wake many sleeping goroutines and timers.
func timers() {
	var wg sync.WaitGroup
	var fired atomic.Int32
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			time.Sleep(time.Duration(i%10) * time.Millisecond)
			fired.Add(1)
		}(i)
		time.AfterFunc(time.Duration(i%7)*time.Millisecond, func() {
			fired.Add(1)
			wg.Done()
		})
	}
	ticker := time.NewTicker(time.Millisecond)
	for i := 0; i < 5; i++ {
		<-ticker.C
	}
	ticker.Stop()
	wg.Wait()
	println("timers:", fired.Load())
}

// Exit while other goroutines still run or block.
func shutdown() {
	for i := 0; i < 4; i++ {
		go func() {
			for {
				runtime.Gosched()
			}
		}()
	}
	go func() {
		select {}
	}()
	println("shutdown")
}
