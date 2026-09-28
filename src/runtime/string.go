package runtime

// This file implements functions related to Go strings.

import (
	"internal/gclayout"
	"unsafe"
)

// The underlying struct for the Go string type.
type _string struct {
	ptr    *byte
	length uintptr
}

// The iterator state for a range over a string.
type stringIterator struct {
	byteindex uintptr
}

const staticBytes = "" +
	"\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f" +
	"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f" +
	"\x20\x21\x22\x23\x24\x25\x26\x27\x28\x29\x2a\x2b\x2c\x2d\x2e\x2f" +
	"\x30\x31\x32\x33\x34\x35\x36\x37\x38\x39\x3a\x3b\x3c\x3d\x3e\x3f" +
	"\x40\x41\x42\x43\x44\x45\x46\x47\x48\x49\x4a\x4b\x4c\x4d\x4e\x4f" +
	"\x50\x51\x52\x53\x54\x55\x56\x57\x58\x59\x5a\x5b\x5c\x5d\x5e\x5f" +
	"\x60\x61\x62\x63\x64\x65\x66\x67\x68\x69\x6a\x6b\x6c\x6d\x6e\x6f" +
	"\x70\x71\x72\x73\x74\x75\x76\x77\x78\x79\x7a\x7b\x7c\x7d\x7e\x7f" +
	"\x80\x81\x82\x83\x84\x85\x86\x87\x88\x89\x8a\x8b\x8c\x8d\x8e\x8f" +
	"\x90\x91\x92\x93\x94\x95\x96\x97\x98\x99\x9a\x9b\x9c\x9d\x9e\x9f" +
	"\xa0\xa1\xa2\xa3\xa4\xa5\xa6\xa7\xa8\xa9\xaa\xab\xac\xad\xae\xaf" +
	"\xb0\xb1\xb2\xb3\xb4\xb5\xb6\xb7\xb8\xb9\xba\xbb\xbc\xbd\xbe\xbf" +
	"\xc0\xc1\xc2\xc3\xc4\xc5\xc6\xc7\xc8\xc9\xca\xcb\xcc\xcd\xce\xcf" +
	"\xd0\xd1\xd2\xd3\xd4\xd5\xd6\xd7\xd8\xd9\xda\xdb\xdc\xdd\xde\xdf" +
	"\xe0\xe1\xe2\xe3\xe4\xe5\xe6\xe7\xe8\xe9\xea\xeb\xec\xed\xee\xef" +
	"\xf0\xf1\xf2\xf3\xf4\xf5\xf6\xf7\xf8\xf9\xfa\xfb\xfc\xfd\xfe\xff"

// Return true iff the strings match.
//
//go:nobounds
func stringEqual(x, y string) bool {
	if len(x) != len(y) {
		return false
	}
	for i := 0; i < len(x); i++ {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// Return true iff x < y.
//
//go:nobounds
func stringLess(x, y string) bool {
	l := len(x)
	if m := len(y); m < l {
		l = m
	}
	for i := 0; i < l; i++ {
		if x[i] < y[i] {
			return true
		}
		if x[i] > y[i] {
			return false
		}
	}
	return len(x) < len(y)
}

// Add two strings together.
func stringConcat(x, y _string) _string {
	if x.length == 0 {
		return y
	} else if y.length == 0 {
		return x
	} else {
		length := x.length + y.length
		buf := alloc(length, gclayout.NoPtrs.AsPtr())
		memcpy(buf, unsafe.Pointer(x.ptr), x.length)
		memcpy(unsafe.Add(buf, x.length), unsafe.Pointer(y.ptr), y.length)
		return _string{ptr: (*byte)(buf), length: length}
	}
}

// Create a string from a []byte slice.
func stringFromBytes(x struct {
	ptr *byte
	len uintptr
	cap uintptr
}) _string {
	if x.len == 0 {
		return _string{}
	}
	if x.len == 1 {
		ptr := unsafe.Add(unsafe.Pointer(unsafe.StringData(staticBytes)), uintptr(*x.ptr))
		return _string{
			ptr:    (*byte)(ptr),
			length: 1,
		}
	}
	buf := alloc(x.len, gclayout.NoPtrs.AsPtr())
	memcpy(buf, unsafe.Pointer(x.ptr), x.len)
	return _string{ptr: (*byte)(buf), length: x.len}
}

// Convert a string to a []byte slice.
func stringToBytes(x _string) (slice struct {
	ptr *byte
	len uintptr
	cap uintptr
}) {
	buf := alloc(x.length, gclayout.NoPtrs.AsPtr())
	memcpy(buf, unsafe.Pointer(x.ptr), x.length)
	slice.ptr = (*byte)(buf)
	slice.len = x.length
	slice.cap = x.length
	return
}

// Convert a []rune slice to a string.
func stringFromRunes(runeSlice []rune) (s _string) {
	// Count the number of characters that will be in the string.
	for _, r := range runeSlice {
		_, numBytes := encodeUTF8(r)
		s.length += numBytes
	}

	// Allocate memory for the string.
	s.ptr = (*byte)(alloc(s.length, gclayout.NoPtrs.AsPtr()))

	// Encode runes to UTF-8 and store the resulting bytes in the string.
	index := uintptr(0)
	for _, r := range runeSlice {
		array, numBytes := encodeUTF8(r)
		for _, c := range array[:numBytes] {
			*(*byte)(unsafe.Add(unsafe.Pointer(s.ptr), index)) = c
			index++
		}
	}

	return
}

// Convert a string to []rune slice.
func stringToRunes(s string) []rune {
	var n = 0
	for range s {
		n++
	}
	var r = make([]rune, n)
	n = 0
	for _, e := range s {
		r[n] = e
		n++
	}
	return r
}

// Create a string from a Unicode code point.
func stringFromUnicode(x rune) _string {
	array, length := encodeUTF8(x)
	// Array will be heap allocated.
	// The heap most likely doesn't work with blocks below 4 bytes, so there's
	// no point in allocating a smaller buffer for the string here.
	return _string{ptr: (*byte)(unsafe.Pointer(&array)), length: length}
}

// Iterate over a string.
// Returns (ok, key, value).
func stringNext(s string, it *stringIterator) (bool, int, rune) {
	if len(s) <= int(it.byteindex) {
		return false, 0, 0
	}
	i := int(it.byteindex)
	r, length := decodeUTF8(s, it.byteindex)
	it.byteindex += length
	return true, i, r
}

// Convert a Unicode code point into an array of bytes and its length.
func encodeUTF8(r rune) ([4]byte, uintptr) {
	x := uint32(r)

	// https://stackoverflow.com/questions/6240055/manually-converting-unicode-codepoints-into-utf-8-and-utf-16
	// Note: this code can probably be optimized (in size and speed).
	switch {
	case x <= 0x7f:
		return [4]byte{byte(x), 0, 0, 0}, 1
	case x <= 0x7ff:
		b1 := 0xc0 | byte(x>>6)
		b2 := 0x80 | byte(x&0x3f)
		return [4]byte{b1, b2, 0, 0}, 2
	case 0xd800 <= x && x <= 0xdfff:
		// utf-16 surrogates are replaced with "invalid code point"
		return [4]byte{0xef, 0xbf, 0xbd, 0}, 3
	case x <= 0xffff:
		b1 := 0xe0 | byte(x>>12)
		b2 := 0x80 | byte((x>>6)&0x3f)
		b3 := 0x80 | byte((x>>0)&0x3f)
		return [4]byte{b1, b2, b3, 0}, 3
	case x <= 0x10ffff:
		b1 := 0xf0 | byte(x>>18)
		b2 := 0x80 | byte((x>>12)&0x3f)
		b3 := 0x80 | byte((x>>6)&0x3f)
		b4 := 0x80 | byte((x>>0)&0x3f)
		return [4]byte{b1, b2, b3, b4}, 4
	default:
		// Invalid Unicode code point.
		return [4]byte{0xef, 0xbf, 0xbd, 0}, 3
	}
}

// Decode a single UTF-8 character from a string.
//
//go:nobounds
func decodeUTF8(s string, index uintptr) (rune, uintptr) {
	remaining := uintptr(len(s)) - index // must be >= 1 before calling this function
	x := s[index]
	switch {
	case x&0x80 == 0x00: // 0xxxxxxx
		return rune(x), 1
	case x&0xe0 == 0xc0: // 110xxxxx
		if remaining < 2 || !isContinuation(s[index+1]) {
			return 0xfffd, 1
		}
		r := (rune(x&0x1f) << 6) | (rune(s[index+1]) & 0x3f)
		if r >= 1<<7 {
			// Check whether the rune really needed to be encoded as a two-byte
			// sequence. UTF-8 requires every rune to be encoded in the smallest
			// sequence possible.
			return r, 2
		}
	case x&0xf0 == 0xe0: // 1110xxxx
		if remaining < 3 || !isContinuation(s[index+1]) || !isContinuation(s[index+2]) {
			return 0xfffd, 1
		}
		r := (rune(x&0x0f) << 12) | ((rune(s[index+1]) & 0x3f) << 6) | (rune(s[index+2]) & 0x3f)
		if r >= 1<<11 && !(r >= 0xD800 && r <= 0xDFFF) {
			// Check whether the rune really needed to be encoded as a
			// three-byte sequence and check that this is not a Unicode
			// surrogate pair (which are not allowed by UTF-8).
			return r, 3
		}
	case x&0xf8 == 0xf0: // 11110xxx
		if remaining < 4 || !isContinuation(s[index+1]) || !isContinuation(s[index+2]) || !isContinuation(s[index+3]) {
			return 0xfffd, 1
		}
		r := (rune(x&0x07) << 18) | ((rune(s[index+1]) & 0x3f) << 12) | ((rune(s[index+2]) & 0x3f) << 6) | (rune(s[index+3]) & 0x3f)
		if r >= 1<<16 && r <= '\U0010FFFF' {
			// Check whether this rune really needed to be encoded as a four
			// byte sequence and check that the resulting rune is in the valid
			// range (up to at most U+10FFFF).
			return r, 4
		}
	}

	// Failed to decode. Return the Unicode replacement character and a length of 1.
	return 0xfffd, 1
}

// isContinuation returns true if (and only if) this is a UTF-8 continuation
// byte.
func isContinuation(b byte) bool {
	// Continuation bytes have their topmost bits set to 0b10.
	return b&0xc0 == 0x80
}

// Functions used in CGo.

// Convert a Go string to a C string.
func cgo_CString(s _string) unsafe.Pointer {
	buf := malloc(s.length + 1)
	memcpy(buf, unsafe.Pointer(s.ptr), s.length)
	*(*byte)(unsafe.Add(buf, s.length)) = 0 // trailing 0 byte
	return buf
}

// Convert a C string to a Go string.
func cgo_GoString(cstr unsafe.Pointer) _string {
	if cstr == nil {
		return _string{}
	}
	return makeGoString(cstr, strlen(cstr))
}

// Convert a C data buffer to a Go string (that possibly contains 0 bytes).
func cgo_GoStringN(cstr unsafe.Pointer, length uintptr) _string {
	return makeGoString(cstr, length)
}

// Make a Go string given a source buffer and a length.
func makeGoString(cstr unsafe.Pointer, length uintptr) _string {
	s := _string{
		length: length,
	}
	if s.length != 0 {
		buf := make([]byte, s.length)
		s.ptr = &buf[0]
		memcpy(unsafe.Pointer(s.ptr), cstr, s.length)
	}
	return s
}

// Convert a C data buffer to a Go byte slice.
func cgo_GoBytes(ptr unsafe.Pointer, length uintptr) []byte {
	// Note: don't return nil if length is 0, to match the behavior of C.GoBytes
	// of upstream Go.
	buf := make([]byte, length)
	if length != 0 {
		memcpy(unsafe.Pointer(unsafe.SliceData(buf)), ptr, uintptr(length))
	}
	return buf
}

func cgo_CBytes(b []byte) unsafe.Pointer {
	p := malloc(uintptr(len(b)))
	s := unsafe.Slice((*byte)(p), len(b))
	copy(s, b)
	return p
}
