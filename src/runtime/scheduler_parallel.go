//go:build scheduler.unified && (exec.multicore || exec.workers)

package runtime

import (
	"runtime/interrupt"
	"sync/atomic"
)

const hasParallelism = true

// The cooperative scheduler is only used in JavaScript.
const asyncScheduler = false

// Set after main.main returns.
var mainExited atomic.Uint32

func schedulerMainExited() bool {
	return mainExited.Load() != 0
}

func lockScheduler() {
	schedulerLock.Lock()
}

func unlockScheduler() {
	schedulerLock.Unlock()
}

// lockTimers takes the scheduler lock, which guards the timer queue and the
// timer node free list.
func lockTimers() interrupt.State {
	schedulerLock.Lock()
	return 0
}

func unlockTimers(mask interrupt.State) {
	schedulerLock.Unlock()
}
