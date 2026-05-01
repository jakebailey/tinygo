//go:build runtime_memhash_wyhash

// Wyhash-inspired hash function, adapted from Go's runtime hash fallback.
// It uses the 32-bit fallback for hash32 and the 64-bit fallback for hash64.

package runtime

import (
	"math/bits"
	"unsafe"
)

const wyhashM5 = 0x1d8e4e27c47d124f

// TinyGo can precompute package-level map literals, so these keys must be
// stable between compile time and run time.
var hashkey = [4]uint64{
	0xa0761d6478bd642f,
	0xe7037ed1a0b428db,
	0x8ebc6af09c88c6e3,
	0x589965cc75374cc3,
}

func hash32(ptr unsafe.Pointer, n, seed uintptr) uint32 {
	a, b := mix32(uint32(seed), uint32(n)^uint32(hashkey[0]))
	if n == 0 {
		return a ^ b
	}
	p := ptr
	s := n
	for ; s > 8; s -= 8 {
		a ^= readU32(p)
		b ^= readU32(unsafe.Add(p, 4))
		a, b = mix32(a, b)
		p = unsafe.Add(p, 8)
	}
	if s >= 4 {
		a ^= readU32(p)
		b ^= readU32(unsafe.Add(p, s-4))
	} else {
		// 1-3 bytes: read without overflowing the buffer.
		t := uint32(*(*byte)(p))
		t |= uint32(*(*byte)(unsafe.Add(p, s>>1))) << 8
		t |= uint32(*(*byte)(unsafe.Add(p, s-1))) << 16
		b ^= t
	}
	a, b = mix32(a, b)
	a, b = mix32(a, b)
	return a ^ b
}

func mix32(a, b uint32) (uint32, uint32) {
	c := uint64(a^uint32(hashkey[1])) * uint64(b^uint32(hashkey[2]))
	return uint32(c), uint32(c >> 32)
}

func readU32(p unsafe.Pointer) uint32 {
	b := (*[4]byte)(p)
	if GOARCH == "mips" {
		return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func hash64(ptr unsafe.Pointer, n, seed uintptr) uint64 {
	var a, b uint64
	s := n
	hseed := uint64(seed) ^ hashkey[0]
	switch {
	case s == 0:
		return hseed
	case s < 4:
		a = uint64(*(*byte)(ptr))
		a |= uint64(*(*byte)(unsafe.Add(ptr, s>>1))) << 8
		a |= uint64(*(*byte)(unsafe.Add(ptr, s-1))) << 16
	case s == 4:
		a = uint64(readU32(ptr))
		b = a
	case s < 8:
		a = uint64(readU32(ptr))
		b = uint64(readU32(unsafe.Add(ptr, s-4)))
	case s == 8:
		a = readU64(ptr)
		b = a
	case s <= 16:
		a = readU64(ptr)
		b = readU64(unsafe.Add(ptr, s-8))
	default:
		p := ptr
		if s > 48 {
			seed1 := hseed
			seed2 := hseed
			for ; s > 48; s -= 48 {
				hseed = mix64(readU64(p)^hashkey[1], readU64(unsafe.Add(p, 8))^hseed)
				seed1 = mix64(readU64(unsafe.Add(p, 16))^hashkey[2], readU64(unsafe.Add(p, 24))^seed1)
				seed2 = mix64(readU64(unsafe.Add(p, 32))^hashkey[3], readU64(unsafe.Add(p, 40))^seed2)
				p = unsafe.Add(p, 48)
			}
			hseed ^= seed1 ^ seed2
		}
		for ; s > 16; s -= 16 {
			hseed = mix64(readU64(p)^hashkey[1], readU64(unsafe.Add(p, 8))^hseed)
			p = unsafe.Add(p, 16)
		}
		a = readU64(unsafe.Add(p, s-16))
		b = readU64(unsafe.Add(p, s-8))
	}

	return mix64(wyhashM5^uint64(n), mix64(a^hashkey[1], b^hseed))
}

func mix64(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	return hi ^ lo
}

func readU64(p unsafe.Pointer) uint64 {
	b := (*[8]byte)(p)
	if GOARCH == "mips" {
		return uint64(b[7]) | uint64(b[6])<<8 | uint64(b[5])<<16 | uint64(b[4])<<24 |
			uint64(b[3])<<32 | uint64(b[2])<<40 | uint64(b[1])<<48 | uint64(b[0])<<56
	}
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}
