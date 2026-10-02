package main

import (
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
	"weak"
)

type node struct {
	next  *node
	value int
	data  [128]byte
}

var (
	parentWeak, childWeak, oldWeak, newWeak weak.Pointer[node]
	parentFieldWeak                         weak.Pointer[int]
	cycleWeak                               [2]weak.Pointer[node]
	resurrected                             atomic.Pointer[node]
	parentDone, childDone, resurrectedDone  atomic.Int32
	cleaned, stopped                        atomic.Int32
	interiorDone                            [3]atomic.Int32
	interiorCleaned                         atomic.Int32
	stackSink                               int
	static                                  node
)

func require(ok bool, message string) {
	if !ok {
		panic(message)
	}
}

//go:noinline
func scrub(depth int) int {
	if depth == 0 {
		return stackSink
	}
	var buf [64]int
	for i := range buf {
		buf[i] = depth + i
	}
	stackSink += buf[depth&63]
	return scrub(depth-1) + buf[0]
}

func collect() {
	stackSink += scrub(40)
	runtime.GC()
	runtime.Gosched()
	fresh(func() {})
	time.Sleep(time.Millisecond)
}

func wait(done func() bool) {
	for i := 0; i < 200; i++ {
		collect()
		if done() {
			return
		}
	}
	println("finalizers:", parentDone.Load(), childDone.Load(), resurrectedDone.Load(), "cleanups:", cleaned.Load())
	panic("collection timed out")
}

//go:noinline
func testIdentity() {
	var zero weak.Pointer[node]
	require(zero == weak.Make[node](nil) && zero.Value() == nil, "nil weak pointer")
	require(unsafe.Sizeof(zero) == unsafe.Sizeof(uintptr(0)), "weak pointer size")
	p := &node{value: 42}
	w := weak.Make(p)
	field := weak.Make(&p.value)
	require(w == weak.Make(p), "weak identity changed")
	require(field == weak.Make(&p.value), "interior weak identity changed")
	require(weak.Make((*byte)(unsafe.Pointer(p))) != weak.Make(&p.data[0]), "offsets share identity")
	runtime.GC()
	require(w.Value() == p && field.Value() == &p.value, "live weak pointer cleared")
	require(w == weak.Make(p), "live weak identity changed after GC")
	runtime.KeepAlive(p)
	s := weak.Make(&static)
	runtime.GC()
	require(s.Value() == &static && s == weak.Make(&static), "static weak pointer")
	z := new(struct{})
	require(weak.Make(z).Value() == z, "zero-sized weak pointer")
}

//go:noinline
func register() {
	child := &node{value: 42}
	parent := &node{next: child}
	parentWeak, childWeak = weak.Make(parent), weak.Make(child)
	parentFieldWeak = weak.Make(&parent.value)
	runtime.SetFinalizer(parent, func(p *node) {
		require(parentWeak.Value() == nil, "parent weak pointer survived finalization")
		require(parentFieldWeak.Value() == nil, "interior weak pointer survived finalization")
		require(childWeak.Value() == p.next && p.next.value == 42, "dependency weak pointer cleared early")
		parentDone.Add(1)
	})
	runtime.SetFinalizer(child, func(*node) {
		require(parentDone.Load() == 1, "child finalized before parent")
		require(childWeak.Value() == nil, "child weak pointer survived finalization")
		childDone.Add(1)
	})
	runtime.AddCleanup(child, func(int) {
		require(childDone.Load() == 1, "cleanup preceded finalizer")
		require(childWeak.Value() == nil, "weak pointer survived cleanup")
		cleaned.Add(1)
	}, 0)

	p := new(node)
	oldWeak = weak.Make(p)
	runtime.SetFinalizer(p, func(p *node) {
		require(oldWeak.Value() == nil, "weak pointer survived resurrection")
		newWeak = weak.Make(p)
		require(newWeak != oldWeak && newWeak.Value() == p, "resurrection reused weak identity")
		resurrected.Store(p)
		resurrectedDone.Add(1)
	})
	runtime.AddCleanup(p, func(int) { cleaned.Add(1) }, 0)

	a, b := new(node), new(node)
	a.next, b.next = b, a
	cycleWeak = [2]weak.Pointer[node]{weak.Make(a), weak.Make(b)}
	for _, p := range []*node{a, b} {
		runtime.AddCleanup(p, func(int) {
			require(cycleWeak[0].Value() == nil && cycleWeak[1].Value() == nil, "cycle weak pointer survived cleanup")
			cleaned.Add(1)
		}, 0)
	}
	c := runtime.AddCleanup(new(node), func(int) { stopped.Add(1) }, 0)
	copy := c
	c.Stop()
	copy.Stop()
}

//go:noinline
func release() {
	p := resurrected.Load()
	require(p != nil && newWeak.Value() == p, "resurrection did not preserve new weak pointer")
	require(oldWeak.Value() == nil, "old weak pointer revived")
	resurrected.Store(nil)
	require(resurrected.Load() == nil, "resurrection root not cleared")
}

//go:noinline
func registerInteriorFinalizers() {
	p := &node{value: 41}
	p.data[1], p.data[2] = 42, 43
	runtime.SetFinalizer(p, func(*node) { panic("cleared base finalizer ran") })
	runtime.SetFinalizer(&p.value, func(*int) { panic("cleared interior finalizer ran") })
	runtime.SetFinalizer(&p.data[1], func(v *byte) {
		require(*v == 42, "wrong interior byte finalizer argument")
		interiorDone[2].Add(1)
	})
	runtime.SetFinalizer(&p.data[2], func(*byte) { panic("cleared byte finalizer ran") })
	runtime.SetFinalizer(&p.value, nil)
	runtime.SetFinalizer(&p.data[2], nil)
	runtime.SetFinalizer(&p.data[3], nil)
	runtime.SetFinalizer(p, nil)
	runtime.SetFinalizer(&p.value, func(v *int) {
		require(*v == 41, "wrong interior int finalizer argument")
		interiorDone[1].Add(1)
	})
	runtime.SetFinalizer(p, func(v *node) {
		require(v.value == 41, "wrong base finalizer argument")
		interiorDone[0].Add(1)
	})
	runtime.AddCleanup(p, func(int) {
		for i := range interiorDone {
			require(interiorDone[i].Load() == 1, "cleanup preceded an interior finalizer")
		}
		interiorCleaned.Add(1)
	}, 0)
}

func main() {
	fresh(testIdentity)
	fresh(registerInteriorFinalizers)
	wait(func() bool { return interiorCleaned.Load() == 1 })
	fresh(register)
	copy := oldWeak
	wait(func() bool {
		return parentDone.Load() == 1 && childDone.Load() == 1 &&
			resurrectedDone.Load() == 1 && cleaned.Load() == 3
	})
	fresh(release)
	wait(func() bool { return cleaned.Load() == 4 })
	require(newWeak.Value() == nil && oldWeak.Value() == nil, "dead weak pointer revived")
	require(newWeak != oldWeak, "dead weak identities changed")
	require(copy == oldWeak && copy.Value() == nil, "copied weak identity changed")
	require(newWeak != weak.Make[node](nil), "dead weak pointer equals zero weak pointer")
	require(stopped.Load() == 0, "stopped cleanup ran")
	testCleanupArguments()
	testPromotion()
	println("ok")
}
