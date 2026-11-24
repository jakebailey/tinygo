package maphash_test

import (
	"hash/maphash"
	"math"
	"strings"
	"testing"
	"unsafe"
)

func TestStreaming(t *testing.T) {
	seed := maphash.MakeSeed()
	prefix := strings.Repeat("a", 127)
	suffix := strings.Repeat("b", 129)
	var h maphash.Hash
	h.SetSeed(seed)
	h.WriteString(prefix)
	h.WriteByte('x')
	clone, err := h.Clone()
	if err != nil {
		t.Fatal(err)
	}
	h.Write([]byte(suffix))
	want := maphash.Bytes(seed, []byte(prefix+"x"+suffix))
	if got := h.Sum64(); got != want {
		t.Fatalf("streamed hash = %x, want %x", got, want)
	}
	if got := clone.(interface{ Sum64() uint64 }).Sum64(); got != maphash.String(seed, prefix+"x") {
		t.Fatalf("cloned hash = %x after writing to original", got)
	}
}

func TestComparableFields(t *testing.T) {
	type key struct {
		_ float64
		x any
		y [2]complex128
	}
	seed := maphash.MakeSeed()
	zero := math.Copysign(0, -1)
	a := key{x: 0.0, y: [2]complex128{1, 2}}
	b := key{x: zero, y: [2]complex128{complex(1, zero), 2}}
	*(*float64)(unsafe.Pointer(&b)) = 42
	if maphash.Comparable(seed, a) != maphash.Comparable(seed, b) {
		t.Fatal("equal composite keys have different hashes")
	}
	b.y[0], b.y[1] = b.y[1], b.y[0]
	if maphash.Comparable(seed, a) == maphash.Comparable(seed, b) {
		t.Fatal("reversing array elements did not change the hash")
	}
}

func TestWriteComparableZeroHash(t *testing.T) {
	var h maphash.Hash
	maphash.WriteComparable(&h, 1)
	got := h.Sum64()
	h.Reset()
	if want := maphash.Comparable(h.Seed(), 1); got != want {
		t.Fatalf("first WriteComparable hash = %x, want %x", got, want)
	}
}

func TestHashComparableAllocations(t *testing.T) {
	seed := maphash.MakeSeed()
	key := struct {
		x int
		s string
	}{42, strings.Repeat("a", 17)}
	if n := testing.AllocsPerRun(10, func() { maphash.Comparable(seed, key) }); n != 0 {
		t.Fatalf("Comparable allocated %v times, want 0", n)
	}
}
