//go:build !gc.custom && !gc.whippet

package runtime

import "unsafe"

func freeManual(ptr unsafe.Pointer) {
	if ptr != nil && ptr != unsafe.Pointer(zeroSizeAllocPtr) {
		free(ptr)
	}
}
