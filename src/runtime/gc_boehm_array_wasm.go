//go:build gc.boehm && tinygo.wasm

package runtime

import "unsafe"

func boehmArrayDescriptor(layout unsafe.Pointer, elementCount, elementSize uintptr) uintptr {
	return 0
}
