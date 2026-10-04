package main

func inlineFastPath(s string, callback func()) (byte, bool) {
	if s != "" {
		return s[0], true
	}
	return inlineExpensiveSlowPath(s, callback)
}

func inlineSlowPath(s string) (byte, bool) {
	return byte(len(s)), false
}

func inlineSingleCall(s string) (byte, bool) {
	return inlineSlowPath(s)
}

type inlineAllocated struct {
	value int
}

func inlineConstructor(value int) *inlineAllocated {
	return &inlineAllocated{value: value}
}

func inlineConstructorWrapper(value int) *inlineAllocated {
	return inlineConstructor(value)
}

func inlineConstructorChecked(value int) *inlineAllocated {
	inlineDisabled()
	return &inlineAllocated{value: value}
}

func inlineConstructorExpensive(value int) *inlineAllocated {
	inlineDisabled()
	inlineDisabled()
	return &inlineAllocated{value: value}
}

func inlineNonAllocatingPointer(value *inlineAllocated) *inlineAllocated {
	return value
}

func inlineComplexSlowPath(s string) (byte, bool) {
	if s == "" {
		return 0, false
	}
	value, ok := inlineSlowPath(s)
	if ok {
		return value, true
	}
	return value + 1, false
}

func inlineExpensiveSlowPath(s string, callback func()) (byte, bool) {
	callback()
	callback()
	return byte(len(s)), false
}

func inlineTooExpensive(s string) int {
	value, _ := inlineSlowPath(s)
	other, _ := inlineSlowPath(s)
	return int(value) + int(other)
}

func inlineRecursive(value int) int {
	if value == 0 {
		return 0
	}
	return inlineRecursive(value - 1)
}

func inlineMutualA(value int) int {
	if value == 0 {
		return 0
	}
	return inlineMutualB(value - 1)
}

func inlineMutualB(value int) int {
	if value == 0 {
		return 0
	}
	return inlineMutualA(value - 1)
}

func inlineWithDefer() {
	defer inlineDisabled()
	inlinePanic()
}

func inlinePanic() {
	panic("inline panic")
}

func inlineWithGo() {
	go inlineDisabled()
}

func inlineWithRecover() any {
	return recover()
}

//go:noinline
func inlineDisabled() {
}

func main() {
	inlineFastPath("", inlineDisabled)
	for i := 0; i < 1; i++ {
		inlineFastPath("", inlineDisabled)
	}
	inlineSingleCall("")
	inlineConstructor(1)
	inlineConstructorWrapper(2)
	inlineConstructorChecked(3)
	inlineConstructorExpensive(4)
	inlineNonAllocatingPointer(nil)
	inlineComplexSlowPath("")
	inlineTooExpensive("")
	inlineRecursive(0)
	inlineMutualA(0)
	inlineWithDefer()
	inlineWithGo()
	inlineWithRecover()
	inlineDisabled()
}
