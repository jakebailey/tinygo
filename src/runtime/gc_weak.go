//go:build gc.conservative || gc.precise || gc.boehm

package runtime

import "unsafe"

// Addresses are complemented to avoid conservative roots; base == 0 is immortal.
// See https://pkg.go.dev/weak#Pointer.
type weakHandle struct {
	next *weakHandle
	obj  uintptr
	base uintptr
}

var weakHandles *weakHandle
var numWeakPointers uintptr

//go:linkname registerWeakPointer weak.runtime_registerWeakPointer
func registerWeakPointer(ptr unsafe.Pointer) unsafe.Pointer {
	gcLock.Lock()
	if n := findWeakPointer(^uintptr(ptr)); n != nil {
		gcLock.Unlock()
		KeepAlive(ptr)
		return unsafe.Pointer(n)
	}
	gcLock.Unlock()
	entry := &weakHandle{obj: ^uintptr(ptr)}
	gcLock.Lock()
	if n := findWeakPointer(entry.obj); n != nil {
		gcLock.Unlock()
		KeepAlive(ptr)
		return unsafe.Pointer(n)
	}
	initWeakPointer(entry, ptr)
	entry.next = weakHandles
	weakHandles = entry
	numWeakPointers++
	gcLock.Unlock()
	KeepAlive(ptr)
	return unsafe.Pointer(entry)
}

func findWeakPointer(addr uintptr) *weakHandle {
	for n := weakHandles; n != nil; n = n.next {
		if n.obj == addr {
			return n
		}
	}
	return nil
}

//go:linkname makeStrongFromWeak weak.runtime_makeStrongFromWeak
func makeStrongFromWeak(handle unsafe.Pointer) unsafe.Pointer {
	gcLock.Lock()
	var ptr unsafe.Pointer
	if addr := (*weakHandle)(handle).obj; addr != 0 {
		ptr = unsafe.Pointer(^addr)
	}
	gcLock.Unlock()
	return ptr
}

func clearWeakPointers(base uintptr) {
	if numWeakPointers == 0 {
		return
	}
	for n := weakHandles; n != nil; n = n.next {
		if n.base == ^base {
			n.obj = 0
		}
	}
}

func scanWeakPointers() {
	if numWeakPointers == 0 {
		return
	}
	prev := &weakHandles
	for n := *prev; n != nil; n = *prev {
		if n.obj != 0 && weakPointerLive(n) {
			prev = &n.next
			continue
		}
		cancelWeakPointer(n)
		n.obj = 0
		*prev = n.next
		n.next = nil
		numWeakPointers--
	}
}
