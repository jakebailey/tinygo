//go:build !scheduler.none

package main

import (
	"runtime"
	"sync/atomic"
	"unsafe"
	"weak"
)

func fresh(fn func()) {
	done := make(chan struct{})
	go func() {
		fn()
		stackSink += scrub(40)
		close(done)
	}()
	<-done
}

func testCleanupArguments() {
	panics := func(fn func()) (panicked bool) {
		defer func() { panicked = recover() != nil }()
		fn()
		return
	}
	require(panics(func() { runtime.AddCleanup((*node)(nil), func(int) {}, 0) }), "nil cleanup target accepted")
	p := new(node)
	require(panics(func() { runtime.AddCleanup(p, func(*node) {}, p) }), "self-retaining cleanup argument accepted")
	require(panics(func() {
		runtime.AddCleanup(p, func(unsafe.Pointer) {}, unsafe.Pointer(p))
	}), "unsafe self-retaining cleanup argument accepted")
	runtime.AddCleanup(p, func(*node) {}, (*node)(nil)).Stop()
	runtime.KeepAlive(p)
}

func testPromotion() {
	var root atomic.Pointer[node]
	var w weak.Pointer[node]
	fresh(func() {
		p := &node{value: 42}
		root.Store(p)
		w = weak.Make(p)
		runtime.SetFinalizer(p, func(p *node) {
			require(w.Value() == nil, "weak pointer survived concurrent finalization")
			p.value = -1
		})
	})
	ready, start, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	for i := 0; i < 4; i++ {
		go func() {
			p := w.Value()
			require(p != nil, "rooted weak pointer cleared")
			ready <- struct{}{}
			<-start
			for i := 0; i < 1000 && p != nil; i++ {
				runtime.Gosched()
				require(p.value == 42, "promoted pointer finalized while live")
				runtime.KeepAlive(p)
				p = w.Value()
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 4; i++ {
		<-ready
	}
	fresh(func() {
		root.Store(nil)
		require(root.Load() == nil, "promotion root not cleared")
	})
	close(start)
	for i := 0; i < 20; i++ {
		collect()
	}
	for i := 0; i < 4; i++ {
		<-done
	}
}
