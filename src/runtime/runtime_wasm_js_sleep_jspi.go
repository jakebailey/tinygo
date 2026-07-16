//go:build wasm && !wasip1 && scheduler.jspi

package runtime

//export tinygo_jspi_sleepTicks
func sleepTicks(d timeUnit)
