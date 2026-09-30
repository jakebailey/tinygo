package runtime

import (
	"internal/reflectlite"
	"unsafe"
)

// AddCleanup calls cleanup(arg) after ptr is no longer reachable.
// See https://pkg.go.dev/runtime#AddCleanup for lifetime requirements.
func AddCleanup[T, S any](ptr *T, cleanup func(S), arg S) Cleanup {
	if ptr == nil {
		panic("runtime.AddCleanup: ptr is nil")
	}
	value := reflectlite.ValueOf(arg)
	if value.Kind() == reflectlite.Pointer || value.Kind() == reflectlite.UnsafePointer {
		argument := value.UnsafePointer()
		if argument == unsafe.Pointer(ptr) {
			panic("runtime.AddCleanup: ptr is equal to arg, cleanup will never run")
		}
		if cleanupArgumentInObject(unsafe.Pointer(ptr), argument) {
			panic("runtime.AddCleanup: ptr is within arg, cleanup will never run")
		}
	}
	c := registerCleanup(unsafe.Pointer(ptr), func() { cleanup(arg) })
	KeepAlive(ptr)
	return c
}
