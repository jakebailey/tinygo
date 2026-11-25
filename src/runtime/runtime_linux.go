//go:build linux

package runtime

//go:linkname runtime_doAllThreadsSyscall syscall.runtime_doAllThreadsSyscall
func runtime_doAllThreadsSyscall(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, err uintptr) {
	// TinyGo links libc and does not track OS threads for cross-thread syscalls,
	// matching Go's ENOTSUP behavior when cgo is linked.
	const enotsup = 95
	return ^uintptr(0), ^uintptr(0), enotsup
}
