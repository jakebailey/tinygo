//go:build (exec.single || exec.workers) && (amd64 || arm64 || 386 || (arm && !cortexm && !avr && !xtensa && !tinygo.riscv) || mips || mipsle)

package task

import _ "unsafe"

// Returns a pointer where the system stack can be stored.
// See task_stack_tinygoriscv.go.
//
//go:linkname runtime_systemStackPtr runtime.systemStackPtr
func runtime_systemStackPtr() *uintptr

func (s *state) resume() {
	swapTask(s.sp, runtime_systemStackPtr())
}

func (s *state) pause() {
	swapTask(*runtime_systemStackPtr(), &s.sp)
}

// SystemStack returns the system stack pointer when called from a task stack.
// The result is not meaningful when called from the system stack.
func SystemStack() uintptr {
	return *runtime_systemStackPtr()
}
