//go:build scheduler.workers

package runtime

// General purpose registers of a worker suspended for the GC.
type workerGCRegs [32]uintptr

//export tinygo_worker_suspend
func tinygo_worker_suspend(thread uintptr, regs *workerGCRegs) uintptr

//export tinygo_worker_resume
func tinygo_worker_resume(thread uintptr)

// Stop all workers other than self. Call with workersLock held.
func stopTheWorld(self *worker) {
	for w := allWorkers; w != nil; w = w.next {
		if w != self {
			w.gcSP = tinygo_worker_suspend(w.thread, &w.gcRegs)
			if w.gcSP == 0 {
				runtimeFatal("failed to stop a worker for the GC")
			}
		}
	}
}

// Let the workers stopped by stopTheWorld continue.
func startTheWorld(self *worker) {
	for w := allWorkers; w != nil; w = w.next {
		if w != self {
			tinygo_worker_resume(w.thread)
		}
	}
}
