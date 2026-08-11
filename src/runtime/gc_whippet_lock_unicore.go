//go:build gc.whippet && tinygo.unicore

package runtime

type whippetMutex struct{}

func (m *whippetMutex) Lock() {
}

func (m *whippetMutex) Unlock() {
}
