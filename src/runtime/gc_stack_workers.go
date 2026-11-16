//go:build scheduler.workers && (gc.conservative || gc.precise || gc.boehm)

package runtime

import "unsafe"

// Whether the world is stopped. Only used by the goroutine running the GC.
var gcWorldStopped bool

// Stop all other workers and scan all stacks and globals. The world stays
// stopped until gcResumeWorld.
func gcMarkReachable() {
	self := currentWorker()
	if !gcWorldStopped {
		// Keep new workers from starting while the world is stopped.
		workersLock.Lock()
		gcWorldStopped = true
		stopTheWorld(self)
	}

	for w := allWorkers; w != nil; w = w.next {
		// Worker structures are not on the GC heap, so mark the goroutines
		// they run explicitly.
		start := uintptr(unsafe.Pointer(&w.task))
		markRoots(start, start+unsafe.Sizeof(w.task))
		start = uintptr(unsafe.Pointer(&w.locked))
		markRoots(start, start+unsafe.Sizeof(w.locked))
		if w != self {
			if unsafe.Sizeof(w.gcRegs) != 0 {
				start = uintptr(unsafe.Pointer(&w.gcRegs))
				markRoots(start, start+unsafe.Sizeof(w.gcRegs))
			}
			scanWorkerStack(w, w.gcSP)
		}
	}
	scanCurrentStack()
	markGlobals()
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
	if !gcWorldStopped {
		return
	}
	gcWorldStopped = false
	startTheWorld(currentWorker())
	workersLock.Unlock()
}
