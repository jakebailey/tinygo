//go:build scheduler.workers

package task

// Affinity holds the worker a task is locked to with LockOSThread.
type Affinity struct {
	// Worker is the address of the runtime worker, or 0 if the task isn't
	// locked to a worker.
	Worker uintptr
}
