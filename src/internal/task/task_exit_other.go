//go:build (scheduler.tasks || scheduler.asyncify || scheduler.cores) && !scheduler.workers

package task

func exitWorker(t *Task) {}
