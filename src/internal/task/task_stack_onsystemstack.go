//go:build (scheduler.tasks || scheduler.cores || scheduler.workers) && !tinygo.riscv

package task

// OnSystemStack returns whether the caller is running on the system stack.
func OnSystemStack() bool {
	// If there is no active goroutine, this must be the system stack.
	return Current() == nil
}
