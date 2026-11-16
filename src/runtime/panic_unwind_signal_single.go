//go:build (tinygo.unwind.explicit || tinygo.unwind.asyncify) && !exec.multicore

package runtime

// The signal is only set while returning synchronously to the defer frame
// recorded in PanicState, and is cleared before deferred calls can schedule.
var unwindPendingSignal bool

//go:inline
func getUnwindSignal() bool {
	return unwindPendingSignal
}

//go:inline
func setUnwindSignal(unwinding bool) {
	unwindPendingSignal = unwinding
}
