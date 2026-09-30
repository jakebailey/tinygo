package main

import (
	"runtime"
	"sync/atomic"
)

type node struct {
	next *node
	n    int
	data [128]byte
}

var (
	ran         [8]atomic.Int32
	finalized   atomic.Int32
	resurrected atomic.Pointer[node]
	handles     []runtime.Cleanup
	stackSink   int
)

func record(id int) {
	ran[id].Add(1)
}

//go:noinline
func register() {
	a, b := new(node), new(node)
	a.next, b.next = b, a
	handles = append(handles, runtime.AddCleanup(a, record, 0))
	runtime.AddCleanup(b, record, 0)

	self := new(node)
	self.next = self
	runtime.AddCleanup(self, record, 1)

	p := new(node)
	runtime.AddCleanup(p, record, 2)
	runtime.AddCleanup(&p.n, record, 2)
	stopped := runtime.AddCleanup(p, record, 3)
	copy := stopped
	stopped.Stop()
	copy.Stop()
	runtime.KeepAlive(p)

	arg := &node{n: 42}
	runtime.AddCleanup(new(node), func(p *node) {
		if p.n != 42 {
			panic("cleanup argument was not kept alive")
		}
		record(4)
	}, arg)

	withFinalizer := new(node)
	runtime.AddCleanup(withFinalizer, func(int) {
		if finalized.Load() != 1 {
			panic("cleanup ran before finalizer")
		}
		record(5)
	}, 0)
	runtime.SetFinalizer(withFinalizer, func(p *node) {
		resurrected.Store(p)
		finalized.Add(1)
		runtime.GC()
		if ran[5].Load() != 0 {
			panic("cleanup ran during finalization")
		}
	})

	captured := new(node)
	runtime.AddCleanup(captured, func(int) {
		captured.n++
		record(6)
	}, 0)

	argRoot := new(node)
	runtime.AddCleanup(argRoot, func(p **node) {
		(*p).n++
		record(7)
	}, &argRoot)
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
	yield()
}

//go:noinline
func releaseResurrected() {
	if resurrected.Load() == nil {
		panic("finalizer did not resurrect its object")
	}
	resurrected.Store(nil)
	if resurrected.Load() != nil {
		panic("resurrection root was not cleared")
	}
}

func main() {
	testCleanupArguments()
	var zero runtime.Cleanup
	zero.Stop()
	onFreshStack(register)
	for i := 0; i < 100; i++ {
		collect()
		if ran[0].Load() == 2 && ran[1].Load() == 1 && ran[2].Load() == 2 &&
			ran[4].Load() == 1 && finalized.Load() == 1 {
			break
		}
	}
	if ran[0].Load() != 2 || ran[1].Load() != 1 || ran[2].Load() != 2 ||
		ran[4].Load() != 1 || finalized.Load() != 1 {
		println(ran[0].Load(), ran[1].Load(), ran[2].Load(), ran[4].Load(), finalized.Load())
		panic("cleanups did not run")
	}
	for i := 0; i < 4; i++ {
		collect()
	}
	if ran[5].Load() != 0 {
		panic("resurrected object was cleaned up")
	}
	onFreshStack(releaseResurrected)
	for i := 0; i < 100 && ran[5].Load() == 0; i++ {
		collect()
	}
	if ran[5].Load() != 1 {
		panic("cleanup did not run after resurrection ended")
	}
	for _, h := range handles {
		h.Stop()
	}
	for i := 0; i < 4; i++ {
		collect()
	}
	if ran[3].Load() != 0 || ran[6].Load() != 0 || ran[7].Load() != 0 {
		panic("stopped or reachable cleanup ran")
	}
	if ran[0].Load() != 2 || ran[1].Load() != 1 || ran[2].Load() != 2 ||
		ran[4].Load() != 1 || ran[5].Load() != 1 {
		panic("cleanup ran more than once")
	}
	testBlockedFinalizer()
	println("ok")
}
