//go:build gc.boehm

package runtime

import "unsafe"

func initWeakPointer(entry *weakHandle, ptr unsafe.Pointer) {
	base := libgc_base(uintptr(ptr))
	if base == 0 {
		return
	}
	entry.base = ^base
	if libgc_register_weak(&entry.obj, unsafe.Pointer(base)) != 0 {
		runtimeFatal("gc: cannot register weak pointer")
	}
	gcResumeWorld()
}

func weakPointerLive(entry *weakHandle) bool {
	return entry.obj != 0
}

func cancelWeakPointer(entry *weakHandle) {
	if entry.base != 0 {
		libgc_unregister_long_link(&entry.obj)
	}
}

//export tinygo_runtime_bdwgc_weak_finalizer
func boehmClearWeakPointers(ptr unsafe.Pointer) {
	clearWeakPointers(uintptr(ptr))
}

//export tinygo_runtime_bdwgc_register_weak
func libgc_register_weak(*uintptr, unsafe.Pointer) int32
