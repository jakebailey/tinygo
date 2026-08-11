//go:build !scheduler.threads

package task

// ThreadsStarted conservatively reports possible parallel execution on
// schedulers that do not use the native threaded scheduler.
func ThreadsStarted() bool {
	return true
}
