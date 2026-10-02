//go:build gc.conservative || gc.precise || gc.boehm

package runtime

import "unsafe"

type cleanupEntry struct {
	next       *cleanupEntry
	obj        uintptr
	fn         func()
	registered bool
}

// Cleanup is a handle to a cleanup call for a specific object.
type Cleanup struct {
	entry *cleanupEntry
}

var (
	cleanups             *cleanupEntry
	cleanupPending       *cleanupEntry
	numCleanups          uintptr
	cleanupsSinceGC      uintptr
	cleanupRunnerStarted bool
	cleanupDraining      bool
)

func cleanupArgumentInObject(ptr, arg unsafe.Pointer) bool {
	gcLock.Lock()
	base, size := cleanupObjectBounds(ptr)
	contains := base != 0 && uintptr(arg) >= base && uintptr(arg)-base < size
	gcLock.Unlock()
	return contains
}

func registerCleanup(ptr unsafe.Pointer, fn func()) Cleanup {
	entry := &cleanupEntry{fn: fn, registered: true}
	gcLock.Lock()
	registered, err := initCleanup(entry, ptr)
	if !registered {
		gcLock.Unlock()
		if err != "" {
			panic(err)
		}
		return Cleanup{}
	}
	entry.next = cleanups
	cleanups = entry
	numCleanups++
	cleanupsSinceGC++
	gcLock.Unlock()
	initCleanupScheduler()
	return Cleanup{entry: entry}
}

// Stop cancels a cleanup that has not yet been queued.
// Keep ptr reachable across Stop to guarantee cancellation.
func (c Cleanup) Stop() {
	if c.entry == nil {
		return
	}
	gcLock.Lock()
	if c.entry.registered {
		prev := &cleanups
		for *prev != nil && *prev != c.entry {
			prev = &(*prev).next
		}
		if *prev == nil {
			runtimeFatal("gc: cleanup missing from registrations")
		}
		cancelCleanup(c.entry)
		*prev = c.entry.next
		c.entry.next = nil
		c.entry.fn = nil
		c.entry.registered = false
		numCleanups--
		if cleanupsSinceGC != 0 {
			cleanupsSinceGC--
		}
	}
	gcLock.Unlock()
}

func scanCleanups() bool {
	cleanupsSinceGC = 0
	if numCleanups == 0 {
		return false
	}
	queued := false
	prev := &cleanups
	for n := *prev; n != nil; n = *prev {
		if !cleanupReady(n) {
			prev = &n.next
			continue
		}
		*prev = n.next
		n.registered = false
		numCleanups--
		n.next = cleanupPending
		cleanupPending = n
		queued = true
	}
	return queued
}

func wakeCleanup() {
	gcLock.Lock()
	if cleanupPending == nil {
		gcLock.Unlock()
		return
	}
	if hasScheduler || hasParallelism {
		spawn := !cleanupRunnerStarted
		if spawn {
			cleanupRunnerStarted = true
		}
		gcLock.Unlock()
		if spawn {
			spawnCleanupRunner()
		}
	} else {
		gcLock.Unlock()
		drainCleanups()
	}
}

func drainCleanups() {
	if cleanupDraining {
		return
	}
	cleanupDraining = true
	for {
		gcLock.Lock()
		n := cleanupPending
		var fn func()
		if n != nil {
			cleanupPending = n.next
			n.next = nil
			fn = n.fn
			n.fn = nil
		}
		gcLock.Unlock()
		if n == nil {
			break
		}
		fn()
	}
	cleanupDraining = false
}

func cleanupRunner() {
	for {
		drainCleanups()
		gcLock.Lock()
		if cleanupPending == nil {
			cleanupRunnerStarted = false
			gcLock.Unlock()
			return
		}
		gcLock.Unlock()
	}
}
