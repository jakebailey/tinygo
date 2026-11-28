//go:build !exec.workers

package runtime

func GOMAXPROCS(n int) int {
	// Note: setting GOMAXPROCS is ignored.
	return 1
}

// Only the workers scheduler needs to know about blocking calls.
func entersyscall() {}

func exitsyscall() {}

// LockOSThread wires the calling goroutine to its current operating system thread.
// Stub for now
// Called by go1.18 standard library on windows, see https://github.com/golang/go/issues/49320
func LockOSThread() {
}

// UnlockOSThread undoes an earlier call to LockOSThread.
// Stub for now
func UnlockOSThread() {
}
