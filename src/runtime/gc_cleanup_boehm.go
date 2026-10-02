//go:build gc.boehm

package runtime

import "unsafe"

func cleanupObjectBounds(ptr unsafe.Pointer) (uintptr, uintptr) {
	base := libgc_base(uintptr(ptr))
	if base == 0 {
		return 0, 0
	}
	return base, libgc_size(base)
}

func initCleanup(entry *cleanupEntry, ptr unsafe.Pointer) (bool, string) {
	base := libgc_base(uintptr(ptr))
	if base == 0 {
		return false, ""
	}
	entry.obj = ^base
	// Long links wait until finalization can no longer resurrect the object.
	// See lib/bdwgc/include/gc/gc.h, GC_register_long_link.
	if libgc_register_long_link(&entry.obj, unsafe.Pointer(base)) != 0 {
		runtimeFatal("gc: cannot register cleanup")
	}
	gcResumeWorld()
	return true, ""
}

func cancelCleanup(entry *cleanupEntry) {
	libgc_unregister_long_link(&entry.obj)
}

func cleanupReady(entry *cleanupEntry) bool {
	return entry.obj == 0
}

//export GC_register_long_link
func libgc_register_long_link(*uintptr, unsafe.Pointer) int32

//export GC_unregister_long_link
func libgc_unregister_long_link(*uintptr) int32

//export GC_get_gc_no
func libgc_get_gc_no() uintptr
