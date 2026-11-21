//go:build scheduler.unified && (exec.single || exec.asyncify || exec.multicore)

package runtime

import "internal/task"

// Mark the given task as runnable. Call with the scheduler lock held.
func schedulerQueue(t *task.Task) {
	runqueue.Push(t)
}

// Remove the next task that may run on this core from the runqueue.
// Call with the scheduler lock held.
func schedulerPop() *task.Task {
	return runqueue.PopCore(currentCPU())
}

// Report whether t may run on this core.
func schedulerCanRun(t *task.Task) bool {
	return t.CanRunOn(currentCPU())
}
