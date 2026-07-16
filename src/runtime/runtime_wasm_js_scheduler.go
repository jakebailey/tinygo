//go:build wasm && !wasi && !scheduler.none && !wasip1 && !wasip2 && !wasm_unknown

package runtime

//export resume
func resume() {
	go func() {
		handleEvent()
	}()

	scheduler(false)
	if mainExited {
		mainReturnExit()
	}
}

//export go_scheduler
func go_scheduler() {
	scheduler(false)
	if mainExited {
		mainReturnExit()
	}
}
