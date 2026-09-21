//go:build (linux || darwin) && scheduler.threads && (386 || amd64 || arm64)

package runtime

import (
	"internal/task"
	"unsafe"
)

//export llvm.frameaddress.p0
func frameAddress(level uint32) unsafe.Pointer

//go:noinline
func Callers(skip int, pc []uintptr) int {
	if len(pc) == 0 {
		return 0
	}

	n := 0
	if skip <= 0 {
		fn := Callers
		pc[n] = (*[2]uintptr)(unsafe.Pointer(&fn))[1]
		n++
		if n == len(pc) {
			return n
		}
	} else {
		skip--
	}

	stackTop := task.StackTop()
	frame := uintptr(frameAddress(0))
	for frame != 0 && frame+2*unsafe.Sizeof(frame) <= stackTop {
		words := (*[2]uintptr)(unsafe.Pointer(frame))
		next := words[0]
		returnPC := words[1]
		if skip == 0 {
			pc[n] = returnPC
			n++
			if n == len(pc) {
				return n
			}
		} else {
			skip--
		}
		if next <= frame || next > stackTop || next%unsafe.Alignof(frame) != 0 {
			break
		}
		frame = next
	}
	return n
}
