//go:build runtime_memhash_tsip

package runtime

import (
	"internal/byteorder"
	"math/bits"
	"unsafe"
)

func hash64(ptr unsafe.Pointer, n uintptr, seed uintptr) uint64 {
	return tsipHash(unsafe.Slice((*byte)(ptr), n), uint64(seed))
}

type sip32 struct {
	v0, v1 uint32
}

func (s *sip32) round() {
	s.v0 += s.v1
	s.v1 = bits.RotateLeft32(s.v1, 5) ^ s.v0
	s.v0 = bits.RotateLeft32(s.v0, 8) + s.v1
	s.v1 = bits.RotateLeft32(s.v1, 13) ^ s.v0
	s.v0 = bits.RotateLeft32(s.v0, 7)
}

func hash32(ptr unsafe.Pointer, n uintptr, seed uintptr) uint32 {
	p := unsafe.Slice((*byte)(ptr), n)

	k0 := uint32(seed)
	k1 := uint32(seed >> 32)

	s := sip32{
		v0: k0 ^ 0x74656462,
		v1: k1 ^ 0x6c796765,
	}
	b := uint32(len(p)) << 24

	for len(p) >= 4 {
		m := byteorder.LEUint32(p[:4])
		s.v1 ^= m
		s.round()
		s.v0 ^= m
		p = p[4:]
	}

	switch len(p) {
	case 3:
		b |= uint32(p[2]) << 16
		fallthrough
	case 2:
		b |= uint32(p[1]) << 8
		fallthrough
	case 1:
		b |= uint32(p[0])
	}

	// last block
	s.v1 ^= b
	s.round()
	s.v0 ^= b

	// finalization
	s.v1 ^= 0xff
	s.round()
	s.v1 = bits.RotateLeft32(s.v1, 16)
	s.round()
	s.v1 = bits.RotateLeft32(s.v1, 16)

	return s.v0 ^ s.v1
}
