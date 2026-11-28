//go:build exec.workers

package task

// exitWorker lets the workers scheduler end the thread of a locked task.
//
//go:linkname exitWorker runtime.workerTaskExit
func exitWorker(t *Task)
