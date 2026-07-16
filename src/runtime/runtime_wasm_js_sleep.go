//go:build wasm && !wasip1 && !scheduler.jspi

package runtime

// This function is called by the scheduler.
// Schedule a call to runtime.scheduler, do not actually sleep.
//
//go:wasmimport gojs runtime.sleepTicks
func sleepTicks(d timeUnit)
