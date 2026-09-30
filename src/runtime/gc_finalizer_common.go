//go:build gc.conservative || gc.precise || gc.boehm

package runtime

import (
	"internal/reflectlite"
	"unsafe"
)

// finalizerGCThreshold starts pressure GC when registrations indicate external memory pressure.
// Larger tables use a proportional threshold. Zero disables this trigger.
const finalizerGCThreshold = 32
const finalizerGCDivisor = 2

// finalizerGCTrigger scales the threshold so scan work stays proportional to registrations.
// It uses finalizerGCThreshold as the minimum.
func finalizerGCTrigger(count uintptr) uintptr {
	if finalizerGCThreshold == 0 {
		return 0
	}
	if proportional := count / finalizerGCDivisor; proportional > finalizerGCThreshold {
		return proportional
	}
	return finalizerGCThreshold
}

func checkFinalizer(obj, fn interface{}) *reflectlite.RawType {
	value := reflectlite.ValueOf(obj)
	if value.Kind() != reflectlite.Pointer {
		runtimeFatal("runtime.SetFinalizer: first argument is not a pointer")
	}
	if value.IsNil() {
		runtimeFatal("runtime.SetFinalizer: pointer not in allocated block")
	}
	if fn != nil {
		f := reflectlite.ValueOf(fn)
		if f.Kind() != reflectlite.Func {
			runtimeFatal("runtime.SetFinalizer: second argument is not a function")
		}
		arg, _ := reflectlite.FinalizerForType(f.RawType())
		if arg == nil || !reflectlite.RawAssignableTo(value.RawType(), arg) {
			runtimeFatal("runtime.SetFinalizer: incompatible finalizer signature")
		}
	}
	return value.RawType()
}

// Use the adapter from compiler.getFinalizerCall for the actual callback ABI.
// Casting to func(unsafe.Pointer) would mishandle interface arguments and results.
func callFinalizer(objPtr unsafe.Pointer, objType *reflectlite.RawType, fn interface{}) {
	var obj interface{}
	*(*_interface)(unsafe.Pointer(&obj)) = _interface{typecode: unsafe.Pointer(objType), value: objPtr}
	fnBox := (*_interface)(unsafe.Pointer(&fn)).value
	_, call := reflectlite.FinalizerForType(reflectlite.ValueOf(fn).RawType())
	call(obj, fnBox)
	KeepAlive(obj)
}
