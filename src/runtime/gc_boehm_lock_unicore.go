//go:build gc.boehm && tinygo.unicore

package runtime

type boehmMutex struct{}

func (m *boehmMutex) Lock() {
}

func (m *boehmMutex) Unlock() {
}
