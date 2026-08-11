//go:build gc.whippet && !tinygo.unicore

package runtime

import "internal/task"

type whippetMutex struct {
	state   uint32
	waiters task.PMutex
}

// Whippet has one shared mutator, so collector entry must be serialized.
// This lock avoids the parking-mutex overhead on its uncontended fast path,
// while allowing all but one contended waiter to sleep.
func (m *whippetMutex) Lock() {
	if libwhippet_mutex_try_lock(&m.state) != 0 {
		return
	}
	m.waiters.Lock()
	libwhippet_mutex_lock(&m.state)
	m.waiters.Unlock()
}

func (m *whippetMutex) Unlock() {
	libwhippet_mutex_unlock(&m.state)
}

//export tinygo_whippet_mutex_try_lock
func libwhippet_mutex_try_lock(state *uint32) uint32

//export tinygo_whippet_mutex_lock
func libwhippet_mutex_lock(state *uint32)

//export tinygo_whippet_mutex_unlock
func libwhippet_mutex_unlock(state *uint32)
