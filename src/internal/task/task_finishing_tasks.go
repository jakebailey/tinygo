//go:build scheduler.tasks || scheduler.jspi

package task

// MarkFinishing does nothing for schedulers that do not use asyncify heap stacks.
func MarkFinishing() {}
