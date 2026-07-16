//go:build scheduler.tasks || scheduler.asyncify || scheduler.cores

package task

func terminate() {
	// TODO: explicitly free the stack after switching back to the scheduler.
	Pause()
}
