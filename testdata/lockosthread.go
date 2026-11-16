package main

// Test LockOSThread with a scheduler that runs goroutines on a pool of threads.

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
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

func lockedSyscall() {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	defer r.Close()
	defer w.Close()

	ready := make(chan struct{})
	done := make(chan bool)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tid := syscall.Gettid()
		close(ready)
		var buf [1]byte
		if _, err := r.Read(buf[:]); err != nil {
			panic(err)
		}
		done <- syscall.Gettid() == tid && buf[0] == 1
	}()
	<-ready
	time.Sleep(10 * time.Millisecond)
	runtime.GC()
	if _, err := w.Write([]byte{1}); err != nil {
		panic(err)
	}
	if !<-done {
		panic("locked goroutine moved during syscall")
	}
	println("locked syscall keeps its thread")
}

func lockedSyscallContention() {
	if runtime.GOMAXPROCS(0) == 1 {
		var active, overlap atomic.Bool
		ready := make(chan struct{})
		done := make(chan bool)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			tid := syscall.Gettid()
			delay := syscall.NsecToTimespec(int64(50 * time.Millisecond))
			runtime.Gosched()
			close(ready)
			for {
				_, _, err := syscall.Syscall(syscall.SYS_NANOSLEEP, uintptr(unsafe.Pointer(&delay)), uintptr(unsafe.Pointer(&delay)), 0)
				if err != syscall.EINTR {
					if err != 0 {
						panic(err)
					}
					break
				}
			}
			if active.Load() {
				overlap.Store(true)
			}
			done <- syscall.Gettid() == tid
		}()
		<-ready
		active.Store(true)
		otherDone := make(chan struct{})
		go func() {
			if active.Load() {
				overlap.Store(true)
			}
			close(otherDone)
		}()
		end := time.Now().Add(150 * time.Millisecond)
		for time.Now().Before(end) {
		}
		active.Store(false)
		if !<-done {
			panic("locked goroutine moved after syscall")
		}
		<-otherDone
		if overlap.Load() {
			panic("syscall return exceeded GOMAXPROCS")
		}
	}
	println("locked syscall waits for a slot")
}

func lockedFairness() {
	var stop atomic.Bool
	var wg sync.WaitGroup
	started := make(chan struct{}, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			started <- struct{}{}
			for !stop.Load() {
				runtime.Gosched()
			}
		}()
	}
	for range 4 {
		<-started
	}
	go func() {
		stop.Store(true)
	}()
	wg.Wait()

	if runtime.GOMAXPROCS(0) == 1 {
		var stopLoop, starved atomic.Bool
		ready := make(chan struct{})
		wake := make(chan struct{})
		pinnedDone := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			close(ready)
			<-wake
			stopLoop.Store(true)
			close(pinnedDone)
		}()
		<-ready
		loopDone := make(chan struct{})
		go func() {
			for !stopLoop.Load() {
				runtime.Gosched()
			}
			close(loopDone)
		}()
		watchdog := time.AfterFunc(250*time.Millisecond, func() {
			starved.Store(true)
			stopLoop.Store(true)
		})
		close(wake)
		<-pinnedDone
		watchdog.Stop()
		<-loopDone
		if starved.Load() {
			panic("runnable goroutine starved a locked goroutine")
		}
	}
	println("locked goroutines share scheduler")
}

func lockedTimer() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	done := make(chan struct{})
	time.AfterFunc(time.Millisecond, func() {
		close(done)
	})
	for {
		select {
		case <-done:
			println("locked goroutine yields to timer")
			return
		default:
			runtime.Gosched()
		}
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

	lockedSyscall()
	lockedSyscallContention()
	lockedFairness()
	lockedTimer()

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
