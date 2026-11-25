package runtime

import (
	"internal/reflectlite"
	"unsafe"
)

type Cleanup struct {
	entry *cleanupEntry
}

func (c Cleanup) Stop() {
	if c.entry == nil {
		return
	}
	stopCleanup(c.entry)
}

func AddCleanup[T, S any](ptr *T, cleanup func(S), arg S) Cleanup {
	if ptr == nil {
		panic("runtime.AddCleanup: ptr is nil")
	}
	if argVal := reflectlite.ValueOf(arg); argVal.Kind() == reflectlite.Pointer || argVal.Kind() == reflectlite.UnsafePointer {
		if unsafe.Pointer(ptr) == *(*unsafe.Pointer)(unsafe.Pointer(&arg)) {
			panic("runtime.AddCleanup: ptr is equal to arg, cleanup will never run")
		}
	}
	return registerCleanup(unsafe.Pointer(ptr), func() {
		cleanup(arg)
	})
}
