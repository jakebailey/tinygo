//go:build scheduler.jspi

package task

import "unsafe"

const stackCanary = uintptr(uint64(0x670c1333b83bf575) & uint64(^uintptr(0)))

//go:linkname runtimePanic runtime.runtimePanic
func runtimePanic(str string)

type state struct {
	entry uintptr
	args  unsafe.Pointer

	csp      unsafe.Pointer
	systemsp unsafe.Pointer

	canaryPtr *uintptr
	next      *Task
	prev      *Task
}

func start(fn uintptr, args unsafe.Pointer, stackSize uintptr) {
	t := &Task{}
	addLiveTask(t)
	t.state.next = jspiTasks
	if jspiTasks != nil {
		jspiTasks.state.prev = t
	}
	jspiTasks = t
	t.state.initialize(fn, args, stackSize)
	scheduleTask(t)
}

var jspiTasks *Task

func complete() {
	t := Current()
	finish(t, false)
	removeJSPITask(t)
}

func removeJSPITask(t *Task) {
	if t.state.prev == nil {
		if jspiTasks != t {
			runtimePanic("JSPI task is not live")
		}
		jspiTasks = t.state.next
	} else {
		t.state.prev.state.next = t.state.next
	}
	if t.state.next != nil {
		t.state.next.state.prev = t.state.prev
	}
	t.state.next = nil
	t.state.prev = nil
}

func terminate() {
	t := Current()
	removeJSPITask(t)
	t.state.exit()
}

//export tinygo_jspi_exit
func (*state) exit()

func (s *state) initialize(fn uintptr, args unsafe.Pointer, stackSize uintptr) {
	s.entry = fn
	s.args = args

	stack := runtime_alloc(stackSize, nil)
	s.canaryPtr = (*uintptr)(stack)
	*s.canaryPtr = stackCanary
	s.csp = unsafe.Add(stack, stackSize)
}

var currentTask *Task

func Current() *Task {
	return currentTask
}

//go:wasmimport gojs runtime.jspiStart
func jspiStart(state uintptr)

func Pause() {
	if *currentTask.state.canaryPtr != stackCanary {
		runtimePanic("stack overflow")
	}
	currentTask.state.pause()
}

//export tinygo_jspi_pause
func (*state) pause()

func (t *Task) Resume() {
	prevTask := currentTask
	if prevTask == nil {
		saveStackPointer()
	}
	t.state.systemsp = unsafe.Pointer(jspiSystemStackPointer())
	t.gcData.swap()
	currentTask = t
	jspiStart(uintptr(unsafe.Pointer(&t.state)))
	if *t.state.canaryPtr != stackCanary {
		runtimePanic("stack overflow")
	}
	currentTask = prevTask
	t.gcData.swap()
}

//go:linkname saveStackPointer runtime.saveStackPointer
func saveStackPointer()

//go:linkname jspiSystemStackPointer runtime.jspiSystemStackPointer
func jspiSystemStackPointer() uintptr

func runJSPI(statePtr uintptr) {
	(*state)(unsafe.Pointer(statePtr)).launch()
}

//export tinygo_jspi_launch
func (*state) launch()

func OnSystemStack() bool {
	return Current() == nil
}
