package runtime

// This file implements Go interfaces.
//
// Interfaces are represented as a pair of {typecode, value}, where value can be
// anything (including non-pointers).

import (
	"internal/gclayout"
	"internal/goarch"
	"internal/reflectlite"
	"unsafe"
)

type _interface struct {
	typecode unsafe.Pointer
	value    unsafe.Pointer
}

// getStaticuint64s is implemented by the compiler as a read-only table.
func getStaticuint64s() *[256]uint64

func staticInteger(bits uint8, size uintptr) unsafe.Pointer {
	ptr := unsafe.Pointer(&getStaticuint64s()[bits])
	if goarch.BigEndian {
		ptr = unsafe.Add(ptr, 8-size)
	}
	return ptr
}

func interfaceInteger8(bits uint8) unsafe.Pointer {
	return staticInteger(bits, 1)
}

//go:inline
func interfaceInteger16(bits uint16) unsafe.Pointer {
	if bits < 256 {
		return staticInteger(uint8(bits), 2)
	}
	ptr := alloc(2, gclayout.NoPtrs.AsPtr())
	*(*uint16)(ptr) = bits
	return ptr
}

//go:inline
func interfaceInteger32(bits uint32) unsafe.Pointer {
	if bits < 256 {
		return staticInteger(uint8(bits), 4)
	}
	ptr := alloc(4, gclayout.NoPtrs.AsPtr())
	*(*uint32)(ptr) = bits
	return ptr
}

//go:inline
func interfaceInteger64(bits uint64) unsafe.Pointer {
	if bits < 256 {
		return staticInteger(uint8(bits), 8)
	}
	ptr := alloc(8, gclayout.NoPtrs.AsPtr())
	*(*uint64)(ptr) = bits
	return ptr
}

func interfaceInteger(bits uint64, size uintptr) unsafe.Pointer {
	switch size {
	case 1:
		return interfaceInteger8(uint8(bits))
	case 2:
		return interfaceInteger16(uint16(bits))
	case 4:
		return interfaceInteger32(uint32(bits))
	case 8:
		return interfaceInteger64(bits)
	default:
		panic("runtime: invalid integer interface size")
	}
}

func composeInterface(typecode, value unsafe.Pointer) _interface {
	return _interface{typecode, value}
}

func decomposeInterface(i _interface) (unsafe.Pointer, unsafe.Pointer) {
	return i.typecode, i.value
}

// decomposeAny splits an interface value into its type code and value word.
//
// The usual way to do this inside the runtime is
// *(*_interface)(unsafe.Pointer(&i)), but that takes the address of a local,
// which escapes and therefore allocates at -opt=0. Reach decomposeInterface by
// linkname instead. interface{} and _interface have the same layout, so the
// call needs no address and no allocation.
//
//go:linkname decomposeAny runtime.decomposeInterface
func decomposeAny(i interface{}) (typecode, value unsafe.Pointer)

// Return true iff both interfaces are equal.
func interfaceEqual(x, y interface{}) bool {
	return reflectValueEqual(reflectlite.ValueOf(x), reflectlite.ValueOf(y))
}

func reflectValueEqual(x, y reflectlite.Value) bool {
	// Note: doing a x.Type() == y.Type() comparison would not work here as that
	// would introduce an infinite recursion: comparing two reflectlite.Type values
	// is done with this reflectValueEqual runtime call.
	if x.RawType() == nil || y.RawType() == nil {
		// One of them is nil.
		return x.RawType() == y.RawType()
	}

	if x.RawType() != y.RawType() {
		// The type is not the same, which means the interfaces are definitely
		// not the same.
		return false
	}

	switch x.RawType().Kind() {
	case reflectlite.Bool:
		return x.Bool() == y.Bool()
	case reflectlite.Int, reflectlite.Int8, reflectlite.Int16, reflectlite.Int32, reflectlite.Int64:
		return x.Int() == y.Int()
	case reflectlite.Uint, reflectlite.Uint8, reflectlite.Uint16, reflectlite.Uint32, reflectlite.Uint64, reflectlite.Uintptr:
		return x.Uint() == y.Uint()
	case reflectlite.Float32, reflectlite.Float64:
		return x.Float() == y.Float()
	case reflectlite.Complex64, reflectlite.Complex128:
		return x.Complex() == y.Complex()
	case reflectlite.String:
		return x.String() == y.String()
	case reflectlite.Chan, reflectlite.Ptr, reflectlite.UnsafePointer:
		return x.UnsafePointer() == y.UnsafePointer()
	case reflectlite.Array:
		for i := 0; i < x.Len(); i++ {
			if !reflectValueEqual(x.Index(i), y.Index(i)) {
				return false
			}
		}
		return true
	case reflectlite.Struct:
		for i := 0; i < x.NumField(); i++ {
			if reflectlite.StructFieldName(x.RawType(), i) == "_" {
				continue
			}
			if !reflectValueEqual(x.Field(i), y.Field(i)) {
				return false
			}
		}
		return true
	case reflectlite.Interface:
		return reflectValueEqual(x.Elem(), y.Elem())
	default:
		runtimePanic(errUncomparable)
		return false // unreachable
	}
}

// interfaceTypeAssert is called when a type assert without comma-ok still
// returns false.
func interfaceTypeAssert(ok bool) {
	if !ok {
		runtimePanic(errTypeAssert)
	}
}

// Implemented in the internal/reflectlite package.
func typeImplementsMethodSet(actualTypeNum, assertedMethodSet unsafe.Pointer) bool

// The following declarations are only used during IR construction. They are
// lowered to inline IR in the interface lowering pass.
// See compiler/interface-lowering.go for details.

type structField struct {
	typecode unsafe.Pointer // type of this struct field
	data     *uint8         // pointer to byte array containing name, tag, varint-encoded offset, and some flags
}

// Pseudo function call used during a type assert. It is used during interface
// lowering, to assign the lowest type numbers to the types with the most type
// asserts. Also, it is replaced with const false if this type assert can never
// happen.
func typeAssert(actualType unsafe.Pointer, assertedType *uint8) bool
