//go:build tinygo.unwind.explicit && exec.multicore

package runtime

var unwindPendingSignal [maxCPU]bool

//go:inline
func getUnwindSignal() bool {
	return unwindPendingSignal[currentCPU()]
}

//go:inline
func setUnwindSignal(unwinding bool) {
	unwindPendingSignal[currentCPU()] = unwinding
}
