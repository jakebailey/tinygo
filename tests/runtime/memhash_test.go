package main

import (
	"hash/maphash"
	"strconv"
	"testing"
	"unsafe"
)

var buf [8192]byte

func BenchmarkMaphash(b *testing.B) {
	var h maphash.Hash
	benchmarkHash(b, "maphash", h)
}

func benchmarkHash(b *testing.B, str string, h maphash.Hash) {
	var sizes = []int{1, 2, 3, 4, 5, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 1024, 8192}
	for _, n := range sizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) { benchmarkHashn(b, int64(n), h) })
	}
}

var total uint64

func TestStringEqualUnaligned(t *testing.T) {
	var a, b [96]byte
	for i := range a {
		a[i] = byte(i + 1)
		b[i] = a[i]
	}

	for offset := 0; offset < 8; offset++ {
		for size := 0; size <= 64; size++ {
			x := unsafe.String(&a[offset], size)
			y := unsafe.String(&b[offset], size)
			if x != y {
				t.Fatalf("equal strings differ: offset=%d size=%d", offset, size)
			}
			if size != 0 {
				b[offset+size-1] ^= 0xff
				y = unsafe.String(&b[offset], size)
				if x == y {
					t.Fatalf("different strings compare equal: offset=%d size=%d", offset, size)
				}
				b[offset+size-1] ^= 0xff
			}
		}
	}
}

func TestSmallMapKeyEqual(t *testing.T) {
	type binaryKey struct {
		a uint8
		b uint16
		c [5]byte
	}
	binaryMap := map[binaryKey]int{
		{a: 1, b: 0x2345, c: [5]byte{1, 2, 3, 4, 5}}: 10,
	}
	if got := binaryMap[binaryKey{a: 1, b: 0x2345, c: [5]byte{1, 2, 3, 4, 5}}]; got != 10 {
		t.Fatalf("lookup with binary key failed: got %d", got)
	}
	if _, ok := binaryMap[binaryKey{a: 1, b: 0x2345, c: [5]byte{1, 2, 3, 4, 6}}]; ok {
		t.Fatal("lookup with different binary key succeeded")
	}

	type mixedKey struct {
		a [5]uint16
		s string
	}
	mixedMap := map[mixedKey]int{
		{a: [5]uint16{1, 2, 3, 4, 5}, s: "value"}: 20,
	}
	if got := mixedMap[mixedKey{a: [5]uint16{1, 2, 3, 4, 5}, s: "value"}]; got != 20 {
		t.Fatalf("lookup with mixed key failed: got %d", got)
	}
	if _, ok := mixedMap[mixedKey{a: [5]uint16{1, 2, 3, 4, 6}, s: "value"}]; ok {
		t.Fatal("lookup with different mixed key succeeded")
	}

	type recursiveBinaryKey struct {
		next *recursiveBinaryKey
		n    uint16
		b    byte
	}
	var sentinel recursiveBinaryKey
	recursiveBinaryMap := map[recursiveBinaryKey]int{
		{next: &sentinel, n: 0x1234, b: 5}: 30,
	}
	if got := recursiveBinaryMap[recursiveBinaryKey{next: &sentinel, n: 0x1234, b: 5}]; got != 30 {
		t.Fatalf("lookup with recursive binary key failed: got %d", got)
	}
	if _, ok := recursiveBinaryMap[recursiveBinaryKey{next: nil, n: 0x1234, b: 5}]; ok {
		t.Fatal("lookup with different recursive binary key succeeded")
	}

	type recursiveMixedKey struct {
		next *recursiveMixedKey
		s    string
	}
	var mixedSentinel recursiveMixedKey
	recursiveMixedMap := map[recursiveMixedKey]int{
		{next: &mixedSentinel, s: "value"}: 40,
	}
	if got := recursiveMixedMap[recursiveMixedKey{next: &mixedSentinel, s: "value"}]; got != 40 {
		t.Fatalf("lookup with recursive mixed key failed: got %d", got)
	}
	if _, ok := recursiveMixedMap[recursiveMixedKey{next: nil, s: "value"}]; ok {
		t.Fatal("lookup with different recursive mixed key succeeded")
	}
}

func benchmarkHashn(b *testing.B, size int64, h maphash.Hash) {
	b.SetBytes(size)

	sum := make([]byte, 4)

	for i := 0; i < b.N; i++ {
		h.Reset()
		h.Write(buf[:size])
		sum = h.Sum(sum[:0])
		total += uint64(sum[0])
	}
}
