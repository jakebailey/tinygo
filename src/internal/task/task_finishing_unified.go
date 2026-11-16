//go:build exec.single || exec.multicore || exec.workers

package task

// MarkFinishing does nothing because goroutine stacks are not asyncify heap stacks.
func MarkFinishing() {}
