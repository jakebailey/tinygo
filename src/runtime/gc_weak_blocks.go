//go:build gc.conservative || gc.precise

package runtime

import "unsafe"

func initWeakPointer(entry *weakHandle, ptr unsafe.Pointer) {
	if base, _ := blockAllocation(uintptr(ptr)); base != 0 {
		entry.base = ^base
	}
}

func weakPointerLive(entry *weakHandle) bool {
	return entry.base == 0 || blockFromAddr(^entry.base).findHead().state() == blockStateMark
}

func cancelWeakPointer(entry *weakHandle) {}
