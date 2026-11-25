// Package maphash computes seeded hashes of byte sequences and comparable Go values.
package maphash

import (
	"hash"
	"unsafe"
)

// A Seed selects a hash function. It is valid only in the current process.
type Seed struct {
	s uint64
}

// MakeSeed returns a new random seed.
func MakeSeed() Seed {
	for {
		if s := randUint64(); s != 0 {
			return Seed{s: s}
		}
	}
}

//go:linkname randUint64 runtime.rand
func randUint64() uint64

//go:linkname hashBlock runtime.tsipHash
//go:noescape
func hashBlock(p []byte, seed uint64) uint64

func (s Seed) check() {
	if s.s == 0 {
		panic("maphash: use of uninitialized Seed")
	}
}

const blockSize = 128

// Bytes returns the seeded hash of b.
func Bytes(seed Seed, b []byte) uint64 {
	seed.check()
	state, tail := hashBlocks(seed.s, b)
	return hashTail(state, tail)
}

// String returns the seeded hash of s.
func String(seed Seed, s string) uint64 {
	return Bytes(seed, unsafe.Slice(unsafe.StringData(s), len(s)))
}

func hashBlocks(state uint64, b []byte) (uint64, []byte) {
	for len(b) >= blockSize {
		state = hashBlock(b[:blockSize], state)
		b = b[blockSize:]
	}
	return state, b
}

func hashTail(state uint64, b []byte) uint64 {
	if len(b) == 0 {
		return state
	}
	return hashBlock(b, state)
}

// A Hash computes a seeded hash. Its zero value chooses a seed on first use.
// A Hash must not be used concurrently. Seeds may be shared across goroutines.
type Hash struct {
	_     [0]func()
	seed  Seed
	state uint64
	buf   [blockSize]byte
	n     int
}

func (h *Hash) initSeed() {
	if h.seed.s == 0 {
		h.seed = MakeSeed()
		h.state = h.seed.s
	}
}

// Seed returns h's seed.
func (h *Hash) Seed() Seed {
	h.initSeed()
	return h.seed
}

// SetSeed sets h's seed and discards any input.
func (h *Hash) SetSeed(seed Seed) {
	seed.check()
	h.seed = seed
	h.Reset()
}

// Reset discards any input and keeps h's seed.
func (h *Hash) Reset() {
	h.initSeed()
	h.state = h.seed.s
	h.n = 0
}

// Write adds b to the hash. It always writes all bytes and never fails.
func (h *Hash) Write(b []byte) (int, error) {
	size := len(b)
	if h.n != 0 {
		n := copy(h.buf[h.n:], b)
		h.n += n
		if h.n != blockSize {
			return size, nil
		}
		h.flush()
		b = b[n:]
	}
	if len(b) >= blockSize {
		h.initSeed()
		h.state, b = hashBlocks(h.state, b)
	}
	h.n = copy(h.buf[:], b)
	return size, nil
}

// WriteString adds the bytes of s to the hash.
func (h *Hash) WriteString(s string) (int, error) {
	return h.Write(unsafe.Slice(unsafe.StringData(s), len(s)))
}

// WriteByte adds b to the hash.
func (h *Hash) WriteByte(b byte) error {
	h.buf[h.n] = b
	h.n++
	if h.n == blockSize {
		h.flush()
	}
	return nil
}

func (h *Hash) flush() {
	h.initSeed()
	h.state = hashTail(h.state, h.buf[:h.n])
	h.n = 0
}

// Sum64 returns the current hash.
func (h *Hash) Sum64() uint64 {
	h.initSeed()
	return hashTail(h.state, h.buf[:h.n])
}

// Sum appends the current hash to b in little-endian order.
func (h *Hash) Sum(b []byte) []byte {
	s := h.Sum64()
	for i := 0; i < 8; i++ {
		b = append(b, byte(s))
		s >>= 8
	}
	return b
}

// Size returns the hash size in bytes.
func (h *Hash) Size() int { return 8 }

// BlockSize returns the byte buffer size.
func (h *Hash) BlockSize() int { return blockSize }

// Clone returns a copy with the same seed and hash state.
func (h *Hash) Clone() (hash.Cloner, error) {
	h.initSeed()
	clone := *h
	return &clone, nil
}
