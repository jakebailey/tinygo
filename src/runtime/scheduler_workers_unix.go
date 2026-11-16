//go:build scheduler.unified && exec.workers && !windows

package runtime

import "internal/futex"

// The GC signal handler runs on the stack of the worker, so the registers of
// the interrupted code are on that stack.
type workerGCRegs struct{}

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

// Stop all workers other than self. Call with workersLock held.
func stopTheWorld(self *worker) {
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

// Let the workers stopped by stopTheWorld continue.
func startTheWorld(self *worker) {
	gcEpoch.Add(1)
	gcEpoch.WakeAll()
}
