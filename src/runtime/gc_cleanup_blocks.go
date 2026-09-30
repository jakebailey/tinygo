//go:build gc.conservative || gc.precise

package runtime

import "unsafe"

func cleanupObjectBounds(ptr unsafe.Pointer) (uintptr, uintptr) {
	base, size, _ := blockAllocation(uintptr(ptr))
	return base, size
}

func initCleanup(entry *cleanupEntry, ptr unsafe.Pointer) (bool, string) {
	addr := uintptr(ptr)
	if !isOnHeap(addr) {
		return false, ""
	}
	block := blockFromAddr(addr)
	if block.state() == blockStateFree {
		return false, "runtime.AddCleanup: ptr not in allocated block"
	}
	head := block.findHead()
	header := (*objHeader)(unsafe.Add(head.pointer(), bytesPerBlock-unsafe.Sizeof(objHeader{})))
	if header.next == 1 {
		return false, "runtime.AddCleanup: manual allocation"
	}
	entry.obj = ^addr
	return true, ""
}

func cancelCleanup(entry *cleanupEntry) {}

func cleanupReady(entry *cleanupEntry) bool {
	return blockFromAddr(^entry.obj).findHead().state() != blockStateMark
}
