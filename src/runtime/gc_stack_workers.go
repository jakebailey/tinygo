//go:build scheduler.workers && (gc.conservative || gc.precise || gc.boehm)

package runtime

import (
	"internal/futex"
	"unsafe"
)

// Odd while the world is stopped. The GC signal handler waits until it changes.
var gcEpoch futex.Futex

// Number of workers that stopped for the current GC cycle.
var gcStopped futex.Futex

//export tinygo_worker_send_gc_signal
func tinygo_worker_send_gc_signal(thread uintptr)

//export tinygo_worker_gc_init
func tinygo_worker_gc_init()

func init() {
	tinygo_worker_gc_init()
}

// Stop all other workers and scan all stacks and globals. The world stays
// stopped until gcResumeWorld.
func gcMarkReachable() {
	self := currentWorker()
	if gcEpoch.Load()&1 == 0 {
		// Keep new workers from starting while the world is stopped.
		workersLock.Lock()
		gcStopped.Store(0)
		gcEpoch.Add(1)
		var n uint32
		for w := allWorkers; w != nil; w = w.next {
			if w != self {
				tinygo_worker_send_gc_signal(w.thread)
				n++
			}
		}
		for {
			stopped := gcStopped.Load()
			if stopped == n {
				break
			}
			gcStopped.Wait(stopped)
		}
	}

	for w := allWorkers; w != nil; w = w.next {
		// Worker structures are not on the GC heap, so mark the goroutine
		// they run explicitly.
		start := uintptr(unsafe.Pointer(&w.task))
		markRoots(start, start+unsafe.Sizeof(w.task))
		if w != self {
			scanWorkerStack(w, w.gcSP)
		}
	}
	scanCurrentStack()
	markGlobals()
}

// Called from the GC signal handler on each other worker.
//
//export tinygo_worker_gc_pause
func workerGCPause() {
	// The handler runs on the interrupted stack, so this frame is below the
	// saved registers of the interrupted code.
	w := currentWorker()
	w.gcSP = uintptr(stacksave())
	epoch := gcEpoch.Load()
	gcStopped.Add(1)
	gcStopped.Wake()
	for gcEpoch.Load() == epoch {
		gcEpoch.Wait(epoch)
	}
}

// Scan the stack of a worker from sp upwards. If the worker is running a
// goroutine, also scan the part of the system stack that is in use.
func scanWorkerStack(w *worker, sp uintptr) {
	if sp >= w.stackBottom && sp <= w.stackTop {
		markRoots(sp, w.stackTop)
		return
	}
	markCurrentGoroutineStack(sp)
	markLiveStack(sp)
	markRoots(w.systemStack, w.stackTop)
}

//go:export tinygo_scanCurrentStack
func scanCurrentStack()

//go:export tinygo_scanstack
func scanstack(sp uintptr) {
	scanWorkerStack(currentWorker(), sp)
}

func gcResumeWorld() {
	if gcEpoch.Load()&1 == 0 {
		// The world wasn't stopped.
		return
	}
	gcEpoch.Add(1)
	gcEpoch.WakeAll()
	workersLock.Unlock()
}
