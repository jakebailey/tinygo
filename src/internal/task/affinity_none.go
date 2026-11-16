//go:build !exec.multicore && !exec.workers

package task

// Affinity is only used by multicore and workers execution modes.
type Affinity struct{}
