//go:build wasm && scheduler.jspi

package runtime

//go:linkname runJSPI internal/task.runJSPI
func runJSPI(state uintptr)

//export tinygo_jspi_run
func jspiRun(state uintptr) {
	runJSPI(state)
}

func jspiSystemStackPointer() uintptr {
	return getCurrentStackPointer()
}
