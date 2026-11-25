//go:build gc.boehm || gc.conservative || gc.precise

package runtime_test

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"weak"
)

type weakTestObj struct {
	a int
	b int
}

var weakScrubSink int

//go:noinline
func scrubWeakStack(depth int) int {
	if depth <= 0 {
		return weakScrubSink
	}
	var buf [64]int
	for i := range buf {
		buf[i] = depth + i
	}
	weakScrubSink += buf[depth&63]
	return scrubWeakStack(depth-1) + buf[0]
}

//go:noinline
func makeDroppedWeak() (weak.Pointer[weakTestObj], weak.Pointer[weakTestObj], weak.Pointer[int]) {
	obj := &weakTestObj{a: 1, b: 2}
	return weak.Make(obj), weak.Make(obj), weak.Make(&obj.a)
}

//go:noinline
func makeDroppedWeakWithFinalizer(ran *atomic.Bool, sawNil *atomic.Bool) weak.Pointer[weakTestObj] {
	obj := &weakTestObj{a: 3, b: 4}
	wp := weak.Make(obj)
	runtime.SetFinalizer(obj, func(*weakTestObj) {
		if wp.Value() == nil {
			sawNil.Store(true)
		}
		ran.Store(true)
	})
	return wp
}

func TestWeakPointer(t *testing.T) {
	var zero weak.Pointer[weakTestObj]
	if zero.Value() != nil {
		t.Fatal("zero weak.Pointer is not nil")
	}
	if weak.Make[weakTestObj](nil) != zero {
		t.Fatal("weak.Make(nil) != zero")
	}

	w1, w2, wField := makeDroppedWeak()
	if w1 != w2 {
		t.Fatal("weak.Make on same pointer produced unequal handles")
	}
	other := &weakTestObj{a: 9}
	wOther := weak.Make(other)
	if w1 == wOther {
		t.Fatal("weak.Make on distinct pointers produced equal handles")
	}
	if wOther.Value() != other {
		t.Fatal("live weak pointer returned wrong value")
	}

	scrubWeakStack(16)
	runtime.GC()
	runtime.GC()

	if v := w1.Value(); v != nil {
		t.Fatalf("weak pointer not cleared after GC: %p", v)
	}
	if v := wField.Value(); v != nil {
		t.Fatalf("interior weak pointer not cleared after GC: %p", v)
	}
	if w1 != w2 {
		t.Fatal("equal weak pointers became unequal after GC")
	}
	if w1 == wOther {
		t.Fatal("distinct weak pointers became equal after GC")
	}
	if wOther.Value() != other {
		t.Fatal("live weak pointer cleared prematurely")
	}
	runtime.KeepAlive(other)
}

func TestWeakPointerFinalizer(t *testing.T) {
	var ran, sawNil atomic.Bool
	wp := makeDroppedWeakWithFinalizer(&ran, &sawNil)

	scrubWeakStack(16)
	runtime.GC()
	if wp.Value() != nil {
		t.Fatal("weak pointer not cleared when finalizer was enqueued")
	}

	deadline := time.Now().Add(2 * time.Second)
	for !ran.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if !ran.Load() {
		t.Fatal("finalizer did not run")
	}
	if !sawNil.Load() {
		t.Fatal("weak pointer was non-nil inside finalizer")
	}
	runtime.GC()
	if wp.Value() != nil {
		t.Fatal("weak pointer non-nil after finalizer ran")
	}
}

//go:noinline
func makeDroppedCleanup(count *atomic.Int32, gotArg *atomic.Int32, stoppedCount *atomic.Int32, sawNil *atomic.Bool) weak.Pointer[weakTestObj] {
	obj := &weakTestObj{a: 5, b: 6}
	wp := weak.Make(obj)
	runtime.AddCleanup(obj, func(v int32) {
		if wp.Value() == nil {
			sawNil.Store(true)
		}
		gotArg.Store(v)
		count.Add(1)
	}, int32(42))
	runtime.AddCleanup(obj, func(v int32) {
		count.Add(v)
	}, int32(10))
	c := runtime.AddCleanup(obj, func(v int32) {
		stoppedCount.Add(v)
	}, int32(1))
	c.Stop()
	return wp
}

func TestAddCleanup(t *testing.T) {
	var zero runtime.Cleanup
	zero.Stop()

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("AddCleanup(nil, ...) did not panic")
			}
		}()
		runtime.AddCleanup((*weakTestObj)(nil), func(int) {}, 0)
	}()

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("AddCleanup(p, ..., p) did not panic")
			}
		}()
		p := &weakTestObj{a: 1}
		runtime.AddCleanup(p, func(*weakTestObj) {}, p)
	}()

	var count, gotArg, stoppedCount atomic.Int32
	var sawNil atomic.Bool
	wp := makeDroppedCleanup(&count, &gotArg, &stoppedCount, &sawNil)

	scrubWeakStack(16)
	runtime.GC()
	runtime.GC()

	deadline := time.Now().Add(2 * time.Second)
	for count.Load() < 11 && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if count.Load() != 11 {
		t.Fatalf("cleanup count = %d, want 11", count.Load())
	}
	if gotArg.Load() != 42 {
		t.Fatalf("cleanup arg = %d, want 42", gotArg.Load())
	}
	if stoppedCount.Load() != 0 {
		t.Fatalf("stopped cleanup ran %d times", stoppedCount.Load())
	}
	if !sawNil.Load() {
		t.Fatal("weak pointer was non-nil inside cleanup")
	}
	if wp.Value() != nil {
		t.Fatal("weak pointer non-nil after cleanup ran")
	}
}
