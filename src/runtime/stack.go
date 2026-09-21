package runtime

import "unsafe"

type Func struct {
	entry uintptr
	name  string
}

type lineEntry struct {
	offset int32
	length uint16
	file   uint16
	line   uint32
}

//go:extern runtime.funcTable
var funcTable *Func

//go:extern runtime.funcTableLen
var funcTableLen uintptr

//go:extern runtime.funcTableSorted
var funcTableSorted bool

//go:extern runtime.lineTable
var lineTable *lineEntry

//go:extern runtime.lineTableLen
var lineTableLen uintptr

//go:extern runtime.lineTableBase
var lineTableBase uintptr

//go:extern runtime.lineFiles
var lineFiles *string

//go:extern runtime.lineFilesLen
var lineFilesLen uintptr

func FuncForPC(pc uintptr) *Func {
	table := unsafe.Slice(funcTable, funcTableLen)
	if !funcTableSorted {
		var best *Func
		for i := range table {
			fn := &table[i]
			if fn.entry <= pc && (best == nil || fn.entry > best.entry) {
				best = fn
			}
		}
		return best
	}
	low, high := 0, len(table)
	for low < high {
		middle := int(uint(low+high) >> 1)
		if table[middle].entry <= pc {
			low = middle + 1
		} else {
			high = middle
		}
	}
	if low == 0 {
		return nil
	}
	return &table[low-1]
}

func (f *Func) Name() string {
	if f == nil {
		return ""
	}
	return f.name
}

func (f *Func) Entry() uintptr {
	if f == nil {
		return 0
	}
	return f.entry
}

func isCallWrapperPC(pc uintptr) bool {
	fn := FuncForPC(pc)
	if fn == nil {
		return false
	}
	name := fn.Name()
	const suffix = "$invoke"
	return len(name) >= len(suffix) && name[len(name)-len(suffix):] == suffix
}

func (f *Func) FileLine(pc uintptr) (file string, line int) {
	if f == nil || lineTableLen == 0 {
		return "", 0
	}
	offset := int64(pc) - int64(lineTableBase)
	table := unsafe.Slice(lineTable, lineTableLen)
	low, high := 0, len(table)
	for low < high {
		middle := int(uint(low+high) >> 1)
		if int64(table[middle].offset) <= offset {
			low = middle + 1
		} else {
			high = middle
		}
	}
	index := low - 1
	if index < 0 {
		return "", 0
	}
	entry := &table[index]
	if uint64(offset-int64(entry.offset)) >= uint64(entry.length) || uintptr(entry.file) >= lineFilesLen {
		return "", 0
	}
	return unsafe.Slice(lineFiles, lineFilesLen)[entry.file], int(entry.line)
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
