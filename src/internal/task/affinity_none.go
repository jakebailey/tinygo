//go:build !scheduler.cores && !scheduler.workers

package task

// Affinity is only used by the cores and workers schedulers.
type Affinity struct{}
