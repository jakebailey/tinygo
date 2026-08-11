//go:build gc.whippet

package runtime

import (
	"internal/gclayout"
	"internal/task"
	"unsafe"
)

const needsStaticHeap = false

const (
	whippetAllocationTagged = iota
	whippetAllocationTaggedPointerless
	whippetAllocationUntaggedConservative
)

var gcLock task.PMutex

func initHeap() {
	libwhippet_init()
}

//go:noinline
func alloc(size uintptr, layout unsafe.Pointer) unsafe.Pointer {
	if size == 0 {
		return alloc_zero(size, layout)
	}

	kind := whippetAllocationTagged
	switch layout {
	case gclayout.NoPtrs.AsPtr():
		kind = whippetAllocationTaggedPointerless
	case gclayout.Conservative.AsPtr():
		kind = whippetAllocationUntaggedConservative
	}

	gcLock.Lock()
	ptr := libwhippet_alloc(size, uintptr(layout), kind)
	whippetResumeWorld()
	gcLock.Unlock()
	if ptr == nil {
		runtimeFatal("gc: out of memory")
	}
	memzero(ptr, size)
	return ptr
}

func allocManual(size uintptr) unsafe.Pointer {
	if size == 0 {
		return alloc_zero(size, gclayout.NoPtrs.AsPtr())
	}
	ptr := libwhippet_manual_alloc(size)
	if ptr == nil {
		runtimeFatal("gc: out of memory")
	}
	return ptr
}

func free(ptr unsafe.Pointer) {
	libwhippet_manual_free(ptr)
}

func freeManual(ptr unsafe.Pointer) {
	if ptr != nil && ptr != unsafe.Pointer(zeroSizeAllocPtr) {
		libwhippet_manual_free(ptr)
	}
}

func GC() {
	gcLock.Lock()
	libwhippet_collect()
	whippetResumeWorld()
	gcLock.Unlock()
}

func ReadMemStats(m *MemStats) {
	gcLock.Lock()
	*m = MemStats{}
	m.TotalAlloc = uint64(libwhippet_allocation_counter())
	m.HeapInuse = uint64(libwhippet_live_size())
	m.HeapSys = uint64(libwhippet_heap_size())
	gcLock.Unlock()
}

func setHeapEnd(newHeapEnd uintptr) {
	runtimeFatal("gc: did not expect setHeapEnd call")
}

func SetFinalizer(obj interface{}, finalizer interface{}) {
}

func markRoots(start, end uintptr) {
	if start < end {
		libwhippet_trace_range(start, end)
	}
}

func markCurrentGoroutineStack(sp uintptr) {
	base := libwhippet_object_base(sp)
	if base == 0 {
		runtimeFatal("goroutine stack not in a heap allocation?")
	}
	markRoots(sp, base+libwhippet_object_size(base))
}

//export tinygo_whippet_trace_roots
func whippetTraceRoots() {
	whippetTracingRoots = true
	gcMarkReachable()
	whippetTracingRoots = false
	whippetWorldStopped = true
}

var whippetTracingRoots bool
var whippetWorldStopped bool

func whippetResumeWorld() {
	if whippetWorldStopped {
		gcResumeWorld()
		whippetWorldStopped = false
	}
}

func markRoot(addr, root uintptr) {
	if whippetTracingRoots {
		libwhippet_trace_range(addr, addr+unsafe.Sizeof(uintptr(0)))
	} else {
		libwhippet_trace_pointer(root)
	}
}

//export tinygo_whippet_init
func libwhippet_init()

//export tinygo_whippet_alloc
func libwhippet_alloc(size, layout uintptr, kind int) unsafe.Pointer

//export tinygo_whippet_collect
func libwhippet_collect()

//export tinygo_whippet_allocation_counter
func libwhippet_allocation_counter() uintptr

//export tinygo_whippet_heap_size
func libwhippet_heap_size() uintptr

//export tinygo_whippet_live_size
func libwhippet_live_size() uintptr

//export tinygo_whippet_trace_pointer
func libwhippet_trace_pointer(value uintptr)

//export tinygo_whippet_trace_range
func libwhippet_trace_range(start, end uintptr)

//export tinygo_whippet_object_base
func libwhippet_object_base(value uintptr) uintptr

//export tinygo_whippet_object_size
func libwhippet_object_size(object uintptr) uintptr

//export tinygo_whippet_manual_alloc
func libwhippet_manual_alloc(size uintptr) unsafe.Pointer

//export tinygo_whippet_manual_free
func libwhippet_manual_free(ptr unsafe.Pointer)
