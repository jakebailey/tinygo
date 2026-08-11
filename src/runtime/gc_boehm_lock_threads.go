//go:build gc.boehm && !tinygo.unicore

package runtime

import "internal/task"

type boehmMutex struct {
	state   uint32
	waiters task.PMutex
}

// Boehm is built without its native thread support, so collector entry must be
// serialized. Avoid the parking-mutex overhead on the uncontended fast path.
func (m *boehmMutex) Lock() {
	if libgc_mutex_try_lock(&m.state) != 0 {
		return
	}
	m.waiters.Lock()
	libgc_mutex_lock(&m.state)
	m.waiters.Unlock()
}

func (m *boehmMutex) Unlock() {
	libgc_mutex_unlock(&m.state)
}

//export tinygo_runtime_bdwgc_mutex_try_lock
func libgc_mutex_try_lock(state *uint32) uint32

//export tinygo_runtime_bdwgc_mutex_lock
func libgc_mutex_lock(state *uint32)

//export tinygo_runtime_bdwgc_mutex_unlock
func libgc_mutex_unlock(state *uint32)
