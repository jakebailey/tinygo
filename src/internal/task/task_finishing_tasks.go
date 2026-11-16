//go:build scheduler.tasks || scheduler.cores || scheduler.workers

package task

// MarkFinishing does nothing because goroutine stacks are not asyncify heap stacks.
func MarkFinishing() {}
