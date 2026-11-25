package maphash

import (
	"internal/reflectlite"
	"math"
	"unsafe"
)

// Comparable returns the seeded hash of v. Equal values have equal hashes.
// A value that is unequal to itself, such as NaN, has a random hash.
func Comparable[T comparable](seed Seed, v T) uint64 {
	return hashComparablePointer(seed.s, &v)
}

// WriteComparable adds v to the hash.
func WriteComparable[T comparable](h *Hash, v T) {
	h.flush()
	h.state = hashComparablePointer(h.state, &v)
}

// The pointer refers to a temporary copy of the value. Only its contents are hashed.
//
//go:linkname hashComparablePointer hash/maphash.hashComparablePointerImpl
//go:noescape
func hashComparablePointer(state uint64, p any) uint64

func hashComparablePointerImpl(state uint64, p any) uint64 {
	return hashComparable(state, reflectlite.ValueOf(p).Elem())
}

func hashWord(state, v uint64) uint64 {
	return hashBlock(unsafe.Slice((*byte)(unsafe.Pointer(&v)), 8), state)
}

func hashComparable(state uint64, v reflectlite.Value) uint64 {
	t := v.RawType()
	if t == nil {
		return hashWord(state, 0)
	}
	if !t.Comparable() {
		panic(unhashableError("hash of unhashable type " + t.String()))
	}
	return hashValue(hashWord(state, uint64(uintptr(unsafe.Pointer(t)))), v)
}

type unhashableError string

func (e unhashableError) Error() string { return string(e) }

func hashFloat(state uint64, f float64) uint64 {
	switch {
	case f == 0:
		return hashWord(state, 0)
	case math.IsNaN(f):
		return hashWord(state, randUint64())
	default:
		return hashWord(state, math.Float64bits(f))
	}
}

func hashValue(state uint64, v reflectlite.Value) uint64 {
	switch v.Kind() {
	case reflectlite.Bool:
		if v.Bool() {
			return hashWord(state, 1)
		}
		return hashWord(state, 0)
	case reflectlite.Int, reflectlite.Int8, reflectlite.Int16, reflectlite.Int32, reflectlite.Int64:
		return hashWord(state, uint64(v.Int()))
	case reflectlite.Uint, reflectlite.Uint8, reflectlite.Uint16, reflectlite.Uint32, reflectlite.Uint64, reflectlite.Uintptr:
		return hashWord(state, v.Uint())
	case reflectlite.Float32, reflectlite.Float64:
		return hashFloat(state, v.Float())
	case reflectlite.Complex64, reflectlite.Complex128:
		c := v.Complex()
		return hashFloat(hashFloat(state, real(c)), imag(c))
	case reflectlite.Chan, reflectlite.Ptr, reflectlite.UnsafePointer:
		// TinyGo stacks and heap objects do not move.
		return hashWord(state, uint64(v.Pointer()))
	case reflectlite.String:
		s := v.String()
		state = hashWord(state, uint64(len(s)))
		return hashBlock(unsafe.Slice(unsafe.StringData(s), len(s)), state)
	case reflectlite.Array:
		for i := 0; i < v.Len(); i++ {
			state = hashValue(state, v.Index(i))
		}
		return state
	case reflectlite.Struct:
		t := v.RawType()
		for i := 0; i < v.NumField(); i++ {
			if reflectlite.StructFieldName(t, i) != "_" {
				state = hashValue(state, v.Field(i))
			}
		}
		return state
	case reflectlite.Interface:
		return hashComparable(state, v.Elem())
	default:
		panic(unhashableError("hash of unhashable type " + v.RawType().String()))
	}
}
