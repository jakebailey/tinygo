//go:build gc.whippet && !tinygo.unicore

package runtime

type whippetMutex struct {
	state uint32
}

// Whippet has one shared mutator, so collector entry must be serialized.
// This lock avoids the parking-mutex overhead on its uncontended fast path.
func (m *whippetMutex) Lock() {
	libwhippet_mutex_lock(&m.state)
}

func (m *whippetMutex) Unlock() {
	libwhippet_mutex_unlock(&m.state)
}

//export tinygo_whippet_mutex_lock
func libwhippet_mutex_lock(state *uint32)

//export tinygo_whippet_mutex_unlock
func libwhippet_mutex_unlock(state *uint32)
