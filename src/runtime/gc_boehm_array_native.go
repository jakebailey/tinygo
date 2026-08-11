//go:build gc.boehm && !tinygo.wasm

package runtime

import "unsafe"

func boehmArrayDescriptor(layout unsafe.Pointer, elementCount, elementSize uintptr) uintptr {
	return libgc_make_array_descriptor(
		uintptr(layout), elementCount, elementSize,
	)
}

//export tinygo_runtime_bdwgc_make_array_descriptor
func libgc_make_array_descriptor(uintptr, uintptr, uintptr) uintptr
