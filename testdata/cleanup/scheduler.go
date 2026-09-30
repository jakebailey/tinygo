//go:build !scheduler.none

package main

import (
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
)

func onFreshStack(fn func()) {
	done := make(chan struct{})
	go func() {
		fn()
		stackSink += scrub(40)
		close(done)
	}()
	<-done
}

func yield() {
	onFreshStack(func() {})
	time.Sleep(time.Millisecond)
}

func testBlockedFinalizer() {
	started := make(chan struct{})
	release := make(chan struct{})
	var childCleaned, independentCleaned atomic.Int32
	onFreshStack(func() {
		child := new(node)
		parent := &node{next: child}
		runtime.AddCleanup(child, func(int) { childCleaned.Add(1) }, 0)
		runtime.SetFinalizer(parent, func(*node) {
			close(started)
			<-release
			if childCleaned.Load() != 0 {
				panic("finalizer dependency cleaned up too soon")
			}
		})
	})
	wait := func(done func() bool) {
		for i := 0; i < 100; i++ {
			collect()
			if done() {
				return
			}
		}
		panic("blocked finalizer test timed out")
	}
	wait(func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	})
	onFreshStack(func() {
		runtime.AddCleanup(new(node), func(int) { independentCleaned.Add(1) }, 0)
	})
	wait(func() bool { return independentCleaned.Load() == 1 })
	if childCleaned.Load() != 0 {
		panic("blocked finalizer dependency was cleaned up")
	}
	close(release)
	wait(func() bool { return childCleaned.Load() == 1 })
}

func panics(fn func()) (panicked bool) {
	defer func() { panicked = recover() != nil }()
	fn()
	return
}

func testCleanupArguments() {
	if !panics(func() { runtime.AddCleanup((*int)(nil), func(int) {}, 0) }) {
		panic("nil cleanup pointer accepted")
	}
	p := new(int)
	if !panics(func() { runtime.AddCleanup(p, func(*int) {}, p) }) {
		panic("cleanup pointer equal to argument accepted")
	}
	if !panics(func() { runtime.AddCleanup(p, func(unsafe.Pointer) {}, unsafe.Pointer(p)) }) {
		panic("cleanup pointer equal to unsafe argument accepted")
	}
	n := new(struct {
		p *byte
		n int
	})
	if !panics(func() { runtime.AddCleanup(n, func(*int) {}, &n.n) }) {
		panic("cleanup argument within target allocation accepted")
	}
	if !panics(func() { runtime.AddCleanup(&n.n, func(unsafe.Pointer) {}, unsafe.Pointer(n)) }) {
		panic("cleanup argument containing target accepted")
	}
	runtime.KeepAlive(n)
}
