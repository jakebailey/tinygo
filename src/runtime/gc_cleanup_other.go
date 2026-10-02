//go:build !gc.conservative && !gc.precise && !gc.boehm

package runtime

import "unsafe"

type Cleanup struct{}

func (c Cleanup) Stop() {}

func cleanupArgumentInObject(ptr, arg unsafe.Pointer) bool {
	return false
}

// Collectors without cleanup support retain the no-op implementation.
// Cleanup execution is not guaranteed: https://pkg.go.dev/runtime#AddCleanup.
func registerCleanup(ptr unsafe.Pointer, fn func()) Cleanup {
	return Cleanup{}
}
