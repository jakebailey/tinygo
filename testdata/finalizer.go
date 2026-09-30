package main

import (
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

type T struct{ x int }

var (
	ranCount     int
	clearedRan   int
	f1Ran        int
	f2Ran        int
	sink         int
	interiorDone [4]atomic.Int32
)

var finalizerTestMode string

// scrubStack overwrites the stack region used by an alloc-and-drop helper with
// non-pointer words. It must be called at the same call depth as that helper so
// this recursion reuses (and clears) the frame that just held the dropped
// pointer; otherwise a stale copy keeps the object marked and it is never
// collected. The returned value derived from buf keeps the writes live.
//
//go:noinline
func scrubStack(depth int) int {
	if depth <= 0 {
		return sink
	}
	var buf [64]int
	for i := range buf {
		buf[i] = depth + i
	}
	sink += buf[depth&63]
	return scrubStack(depth-1) + buf[0]
}

// allocAndDrop allocates an object, registers a finalizer, and returns without
// leaking any reference to it, so the object becomes unreachable. The finalizer
// must not capture the object (that would pin it forever): it takes the pointer
// as its argument and touches only a package global.
//
//go:noinline
func allocAndDrop() {
	p := &T{x: 42}
	runtime.SetFinalizer(p, func(*T) { ranCount++ })
}

//go:noinline
func allocRegisterClear() {
	p := &T{x: 1}
	runtime.SetFinalizer(p, func(*T) { clearedRan++ })
	runtime.SetFinalizer(p, nil)
}

//go:noinline
func allocRegisterReplace() {
	p := &T{x: 2}
	runtime.SetFinalizer(p, func(*T) { f1Ran++ })
	runtime.SetFinalizer(p, nil)
	runtime.SetFinalizer(p, func(*T) { f2Ran++ })
}

// testFires checks that a finalizer runs after its object is collected, and
// only once. scrubStack and the alloc helper are both called here, at the same
// depth, so the scrub clears the helper's stale frame. Gosched lets the
// dedicated finalizer goroutine drain (a no-op under scheduler=none, where
// finalizers already ran inline during GC).
func testFires() {
	allocAndDrop()
	for i := 0; i < 100 && ranCount == 0; i++ {
		sink += scrubStack(40)
		runtime.GC()
		runtime.Gosched()
	}
	if ranCount == 0 {
		panic("finalizer: never ran after object became unreachable")
	}
	for i := 0; i < 100; i++ {
		sink += scrubStack(40)
		runtime.GC()
		runtime.Gosched()
	}
	if ranCount != 1 {
		panic("finalizer: ran more than once")
	}
}

// testClear checks that SetFinalizer(obj, nil) removes a finalizer.
func testClear() {
	allocRegisterClear()
	for i := 0; i < 100; i++ {
		sink += scrubStack(40)
		runtime.GC()
		runtime.Gosched()
	}
	if clearedRan != 0 {
		panic("finalizer: ran after being cleared with nil")
	}
}

func testReplace() {
	allocRegisterReplace()
	for i := 0; i < 100 && f2Ran == 0; i++ {
		sink += scrubStack(40)
		runtime.GC()
		runtime.Gosched()
	}
	if f1Ran != 0 {
		panic("finalizer: replaced finalizer f1 still ran")
	}
	if f2Ran != 1 {
		panic("finalizer: replacement finalizer f2 did not run exactly once")
	}
}

type interiorObject struct {
	pad   [128]byte
	value int
	data  [8]byte
}

type interiorValue uint16

//go:noinline
func registerInteriorFinalizers() {
	p := &interiorObject{value: 41}
	p.data[1] = 42
	runtime.SetFinalizer(p, func(*interiorObject) { panic("cleared base finalizer ran") })
	runtime.SetFinalizer(&p.value, func(*int) { panic("cleared interior finalizer ran") })
	runtime.SetFinalizer(&p.data[1], func(v *byte) {
		if *v != 42 {
			panic("wrong interior byte finalizer argument")
		}
		interiorDone[2].Add(1)
	})
	runtime.SetFinalizer(&p.data[2], func(*byte) { panic("cleared byte finalizer ran") })
	runtime.SetFinalizer(&p.value, nil)
	runtime.SetFinalizer(&p.data[2], nil)
	runtime.SetFinalizer(&p.data[3], nil)
	runtime.SetFinalizer(p, nil)
	runtime.SetFinalizer(&p.value, func(v *int) {
		if *v != 41 {
			panic("wrong interior int finalizer argument")
		}
		interiorDone[1].Add(1)
	})
	runtime.SetFinalizer(p, func(v *interiorObject) {
		if v.value != 41 {
			panic("wrong base finalizer argument")
		}
		interiorDone[0].Add(1)
	})
	q := new(struct {
		pad   [128]byte
		value interiorValue
	})
	q.value = 43
	runtime.SetFinalizer(&q.value, func(v *interiorValue) {
		if *v != 43 {
			panic("wrong named interior finalizer argument")
		}
		interiorDone[3].Add(1)
	})
}

func testInteriorFinalizers() {
	registerInteriorFinalizers()
	for i := 0; i < 200; i++ {
		sink += scrubStack(40)
		runtime.GC()
		runtime.Gosched()
		if runtime.GOARCH != "wasm" {
			time.Sleep(time.Millisecond)
		}
		done := true
		for j := range interiorDone {
			done = done && interiorDone[j].Load() == 1
		}
		if done {
			return
		}
	}
	panic("interior finalizers did not run exactly once")
}

func main() {
	if finalizerTestMode == "interior" {
		testInteriorFinalizers()
		println("ok")
		return
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "interior":
			testInteriorFinalizers()
		case "types":
			testFinalizerTypes()
		default:
			testInvalidFinalizer(os.Args[1])
		}
		println("ok")
		return
	}
	testFires()
	testClear()
	testReplace()
	println("ok")
}

type object struct {
	value int
	data  [128]byte
}

func (p *object) read() int { return p.value }

func (p *object) finish() { check(p) }

type reader interface{ read() int }
type pointer *object
type finalizer func(*object) [2048]int

var completed atomic.Int32

func check(p *object) {
	if p.value != 42 {
		panic("wrong finalizer argument")
	}
	completed.Add(1)
}

//go:noinline
func register(i int) {
	p := &object{value: 42}
	switch i {
	case 0:
		runtime.SetFinalizer(p, func(p *object) { check(p) })
	case 1:
		runtime.SetFinalizer(pointer(p), func(p *object) { check(p) })
	case 2:
		runtime.SetFinalizer(p, func(p pointer) { check(p) })
	case 3:
		runtime.SetFinalizer(p, func(v interface{}) [4]int64 {
			check(v.(*object))
			return [4]int64{}
		})
	case 4:
		runtime.SetFinalizer(p, func(v reader) (int, string) {
			if v.read() != 42 {
				panic("wrong finalizer interface")
			}
			check(v.(*object))
			return 1, "ignored"
		})
	case 5:
		runtime.SetFinalizer(p, finalizer(func(v *object) [2048]int {
			check(v)
			return [2048]int{}
		}))
	case 6:
		runtime.SetFinalizer(p, func(v *object) {
			check(v)
			runtime.SetFinalizer(v, func(v *object) { check(v) })
		})
	case 7:
		runtime.SetFinalizer(p, (*object).finish)
	}
}

func testFinalizerTypes() {
	for i := 0; i < 8; i++ {
		register(i)
	}
	for i := 0; i < 100 && completed.Load() != 9; i++ {
		sink += scrubStack(40)
		runtime.GC()
		runtime.Gosched()
		if runtime.GOARCH != "wasm" {
			time.Sleep(time.Millisecond)
		}
	}
	if completed.Load() != 9 {
		println("completed:", completed.Load())
		panic("finalizer call adapters did not run")
	}
}

func testInvalidFinalizer(name string) {
	p := new(object)
	switch name {
	case "duplicate":
		runtime.SetFinalizer(p, func(*object) {})
		runtime.SetFinalizer(p, func(*object) {})
	case "nil":
		runtime.SetFinalizer((*object)(nil), func(*object) {})
	case "count":
		runtime.SetFinalizer(p, func() {})
	case "variadic":
		runtime.SetFinalizer(p, func(...*object) {})
	case "type":
		runtime.SetFinalizer(p, func(*int) {})
	case "non-function":
		runtime.SetFinalizer(p, 1)
	case "non-pointer":
		runtime.SetFinalizer(1, func(int) {})
	case "interior-pointer":
		q := new(struct {
			pad    [128]byte
			target *object
		})
		runtime.SetFinalizer(&q.target, func(**object) {})
		runtime.KeepAlive(q)
	case "interior-large":
		q := new(struct {
			pad    [128]byte
			target [16]byte
		})
		runtime.SetFinalizer(&q.target, func(*[16]byte) {})
		runtime.KeepAlive(q)
	case "interior-duplicate":
		q := new([128]byte)
		runtime.SetFinalizer(&q[1], func(*byte) {})
		runtime.SetFinalizer(&q[2], func(*byte) {})
		runtime.SetFinalizer(&q[2], nil)
		runtime.SetFinalizer(&q[1], func(*byte) {})
		runtime.KeepAlive(q)
	}
	runtime.KeepAlive(p)
}
