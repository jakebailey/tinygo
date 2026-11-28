//go:build (exec.single || exec.asyncify || exec.multicore) && !exec.workers

package task

func exitWorker(t *Task) {}
