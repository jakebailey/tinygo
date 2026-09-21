package runtime

type Frames struct {
	callers []uintptr
}

type Frame struct {
	PC uintptr

	Func *Func

	Function string

	File string
	Line int

	Entry uintptr
}

func CallersFrames(callers []uintptr) *Frames {
	return &Frames{callers: callers}
}

func (ci *Frames) Next() (frame Frame, more bool) {
	if len(ci.callers) == 0 {
		return Frame{}, false
	}

	pc := ci.callers[0]
	ci.callers = ci.callers[1:]
	fn := FuncForPC(pc)
	if fn != nil && pc > fn.Entry() {
		pc--
		fn = FuncForPC(pc)
	}
	frame.PC = pc
	frame.Func = fn
	if fn != nil {
		frame.Function = fn.Name()
		frame.Entry = fn.Entry()
		frame.File, frame.Line = fn.FileLine(pc)
	}
	return frame, len(ci.callers) != 0
}
