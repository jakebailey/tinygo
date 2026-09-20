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
	inlineComplexSlowPath("")
	inlineTooExpensive("")
	inlineRecursive(0)
	inlineMutualA(0)
	inlineWithDefer()
	inlineWithGo()
	inlineWithRecover()
	inlineDisabled()
}
