//go:build !scheduler.workers

package runtime

func GOMAXPROCS(n int) int {
	// Note: setting GOMAXPROCS is ignored.
	return 1
}

// Only the workers scheduler needs to know about blocking calls.
func entersyscall() {}

func exitsyscall() {}
