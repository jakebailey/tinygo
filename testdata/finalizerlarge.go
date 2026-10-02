package main

import (
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

type largeFinalizerObject struct {
	data [128]byte
}

var largeFinalizerRan bool
var largeFinalizerSink int
var runnerCallbackDone, runnerPayloadDone atomic.Uint32
var finalizerLargeMode string

//go:noinline
func registerRunnerPayload() {
	payload := new(largeFinalizerObject)
	payload.data[0] = 42
	runtime.SetFinalizer(payload, func(*largeFinalizerObject) { runnerPayloadDone.Add(1) })
	target := new(largeFinalizerObject)
	runtime.SetFinalizer(target, func(*largeFinalizerObject) {
		if payload.data[0] != 42 {
			panic("finalizer lost its callback payload")
		}
		runnerCallbackDone.Add(1)
	})
}

func testRunnerReleasesPayload() {
	registerRunnerPayload()
	for i := 0; i < 200 && (runnerCallbackDone.Load() != 1 || runnerPayloadDone.Load() != 1); i++ {
		largeFinalizerSink += scrubLargeFinalizerStack(40)
		runtime.GC()
		runtime.Gosched()
		if runtime.GOARCH != "wasm" {
			time.Sleep(time.Millisecond)
		}
	}
	if runnerCallbackDone.Load() != 1 || runnerPayloadDone.Load() != 1 {
		panic("finalizer runner retained its callback payload")
	}
}

//go:noinline
func scrubLargeFinalizerStack(depth int) int {
	if depth == 0 {
		return largeFinalizerSink
	}
	var buf [64]int
	for i := range buf {
		buf[i] = depth + i
	}
	largeFinalizerSink += buf[depth&63]
	return scrubLargeFinalizerStack(depth-1) + buf[0]
}

//go:noinline
func registerLargeFinalizer() {
	p := new(largeFinalizerObject)
	payload := new(largeFinalizerObject)
	for i := range payload.data {
		payload.data[i] = byte(i)
	}
	runtime.SetFinalizer(p, func(*largeFinalizerObject) {
		for i, value := range payload.data {
			if value != byte(i) {
				panic("finalizer closure data was collected")
			}
		}
		largeFinalizerRan = true
	})
}

func main() {
	mode := finalizerLargeMode
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if mode == "graph" {
		testFinalizerGraph()
		println("ok")
		return
	}
	registerLargeFinalizer()
	for i := 0; i < 100 && !largeFinalizerRan; i++ {
		largeFinalizerSink += scrubLargeFinalizerStack(40)
		runtime.GC()
		runtime.Gosched()
	}
	if !largeFinalizerRan {
		panic("large object finalizer did not run")
	}
	if mode == "runner-payload" {
		testRunnerReleasesPayload()
	}
	println("ok")
}

type graphNode struct {
	next *graphNode
	tag  uint32
	data [128]byte
}

var (
	graphFinalized atomic.Int32
	childFinalized atomic.Int32
	cycleFinalized atomic.Int32
	cycleCleaned   atomic.Int32
	graphKeepAlive []*graphNode
)

//go:noinline
func registerGraphFinalizer() {
	child := &graphNode{tag: 0x12345678}
	parent := &graphNode{next: child}
	runtime.SetFinalizer(child, func(p *graphNode) {
		p.tag = 0
		childFinalized.Add(1)
	})
	runtime.SetFinalizer(parent, func(p *graphNode) {
		runtime.GC()
		for i := 0; i < 8192; i++ {
			graphKeepAlive = append(graphKeepAlive, &graphNode{tag: uint32(i)})
		}
		if p.next.tag != 0x12345678 {
			panic("finalizer lost a referenced object")
		}
		graphFinalized.Add(1)
	})
}

//go:noinline
func registerCycleFinalizers() {
	first := new(graphNode)
	second := new(graphNode)
	first.next = second
	second.next = first
	runtime.SetFinalizer(first, func(*graphNode) { cycleFinalized.Add(1) })
	runtime.SetFinalizer(second, func(*graphNode) { cycleFinalized.Add(1) })
	runtime.AddCleanup(first, func(int) { cycleCleaned.Add(1) }, 0)
	runtime.AddCleanup(second, func(int) { cycleCleaned.Add(1) }, 0)
}

func testFinalizerGraph() {
	registerGraphFinalizer()
	registerCycleFinalizers()
	for i := 0; i < 100 && (graphFinalized.Load() != 1 || childFinalized.Load() != 1); i++ {
		largeFinalizerSink += scrubLargeFinalizerStack(40)
		runtime.GC()
		runtime.Gosched()
		if runtime.GOARCH != "wasm" {
			time.Sleep(time.Millisecond)
		}
	}
	if graphFinalized.Load() != 1 || childFinalized.Load() != 1 {
		panic("finalizers did not run in dependency order")
	}
	if cycleFinalized.Load() != 0 || cycleCleaned.Load() != 0 {
		panic("finalized a cycle without a dependency order")
	}
}
