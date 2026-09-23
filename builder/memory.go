package builder

import "runtime/debug"

func releaseUnusedMemory() {
	debug.FreeOSMemory()
	trimHeap()
}
