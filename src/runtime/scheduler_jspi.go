//go:build scheduler.jspi

package runtime

const jspiScheduler = true

//go:wasmimport gojs runtime.scheduleTimeout
func scheduleScheduler(d timeUnit)
