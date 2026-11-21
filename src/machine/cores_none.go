//go:build !exec.multicore

package machine

// AssignCore assigns the calling goroutine to core until it exits. Without the
// multicore execution only core 0 is valid.
//
// Experimental: this API may change or be removed in a future release.
func AssignCore(core int) {
	if core != 0 {
		panic("machine: core out of range")
	}
}
