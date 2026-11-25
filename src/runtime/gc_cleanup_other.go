//go:build !gc.boehm && !gc.conservative && !gc.precise

package runtime

import "unsafe"

type cleanupEntry struct{}

func registerCleanup(ptr unsafe.Pointer, fn func()) Cleanup {
	return Cleanup{}
}

func stopCleanup(entry *cleanupEntry) {}
