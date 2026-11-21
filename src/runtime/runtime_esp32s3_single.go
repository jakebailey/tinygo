//go:build esp32s3 && !exec.multicore

package runtime

import "runtime/interrupt"

const maxCPU = 2

//go:inline
func currentCPU() uint32 {
	return 0
}

//go:inline
func lockTicks() interrupt.State {
	return 0
}

//go:inline
func unlockTicks(interrupt.State) {
}

//go:inline
func sleepWoken(core uint32) bool {
	return false
}

func enableInterruptOnCore0(num uint32) {
}

func esp32s3EnterFlashSafeSection() (interrupt.State, bool) {
	return interrupt.Disable(), false
}

func esp32s3ExitFlashSafeSection(state interrupt.State, _ bool) {
	interrupt.Restore(state)
}
