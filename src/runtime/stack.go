package runtime

type Func struct {
}

func FuncForPC(pc uintptr) *Func {
	return nil
}

func (f *Func) Name() string {
	return ""
}

func (f *Func) FileLine(pc uintptr) (file string, line int) {
	return "", 0
}

func Caller(skip int) (pc uintptr, file string, line int, ok bool) {
	var pcs [1]uintptr
	if Callers(skip+2, pcs[:]) == 0 {
		return 0, "", 0, false
	}
	frame, _ := CallersFrames(pcs[:]).Next()
	return frame.PC, frame.File, frame.Line, frame.PC != 0
}

func Stack(buf []byte, all bool) int {
	return 0
}
