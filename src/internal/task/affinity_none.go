//go:build !exec.multicore && !exec.workers

package task

// Affinity is unused when tasks cannot be pinned to a core or worker.
type Affinity struct{}

func (t *Task) CanRunOn(uint32) bool {
	return true
}

func (q *Queue) PopCore(uint32) *Task {
	return q.Pop()
}
