//go:build go1.27

package maphash

// Hasher hashes values and defines which values are equal.
// Hash must add the same input to h for values that Equal considers equal.
type Hasher[T any] interface {
	Hash(h *Hash, v T)
	Equal(x, y T) bool
}

// ComparableHasher uses Go equality and comparable-value hashing.
type ComparableHasher[T comparable] struct{}

// Hash adds v to h.
func (ComparableHasher[T]) Hash(h *Hash, v T) {
	WriteComparable(h, v)
}

// Equal reports whether x and y are equal.
func (ComparableHasher[T]) Equal(x, y T) bool {
	return x == y
}
