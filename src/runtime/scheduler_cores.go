//go:build scheduler.cores

package runtime

import (
	"internal/task"
	"runtime/interrupt"
	"sync/atomic"
)

// Non-zero when secondary cores may enter the scheduler.
var secondaryCoresReady atomic.Uint32

func waitForSecondaryCoresReady() {
	for secondaryCoresReady.Load() == 0 {
	}
}

// Which task is running on a given core (or nil if there is no task running on
// the core).
var cpuTasks [numCPU]*task.Task

// NumCPU returns the number of CPU cores on this system.
func NumCPU() int {
	return numCPU
}

// Wake a core that may run the given task.
// Must be called with the scheduler lock held.
func schedulerWakeTask(t *task.Task) {
	if t.Affinity.Core != 0 {
		schedulerWakeCore(uint32(t.Affinity.Core - 1))
	} else {
		schedulerWake()
	}
}

//go:linkname assignCore machine.assignCore
func assignCore(core int) {
	if core < 0 || core >= numCPU {
		panic("machine: core out of range")
	}
	t := task.Current()
	if t.Affinity.Core != 0 {
		panic("machine: AssignCore called twice")
	}
	if core != int(currentCPU()) && secondaryCoresReady.Load() == 0 {
		panic("machine: AssignCore called before all cores have started")
	}
	t.Affinity.Core = uint8(core) + 1
	if core == int(currentCPU()) {
		return
	}

	// Move to the target core.
	schedulerLock.Lock()
	runqueue.Push(t)
	schedulerWakeCore(uint32(core))
	task.PauseLocked()
}

// This function is called on the first core in the system. It will wake up the
// other cores when ready.
func run() {
	initRand()
	initHeap()

	go func() {
		// Package initializers are currently run single-threaded.
		// This might help with registering interrupts and such.
		initAll()

		// After package initializers have finished, start all the other cores.
		startSecondaryCores()
		secondaryCoresReady.Store(1)

		// Run main.main.
		callMain()

		// main.main has exited, so the program should exit.
		mainExited.Store(1)
	}()

	// The scheduler must always be entered while the scheduler lock is taken.
	schedulerLock.Lock()
	scheduler(false)
	schedulerLock.Unlock()
}

// Maximum number of cores that run the scheduler at the same time.
const maxSchedulerThreads = numCPU

// Every core always runs the scheduler.
func schedulerAcquireSlot() {}

// Remove the next task that may run on this core from the runqueue.
// Call with the scheduler lock held.
func schedulerPop() *task.Task {
	return runqueue.PopCore(currentCPU())
}

// Report whether t may run on this core.
func schedulerCanRun(t *task.Task) bool {
	return t.CanRunOn(currentCPU())
}

func currentTask() *task.Task {
	return cpuTasks[currentCPU()]
}

func setCurrentTask(task *task.Task) {
	cpuTasks[currentCPU()] = task
}

func lockFutex() interrupt.State {
	mask := interrupt.Disable()
	futexLock.Lock()
	return mask
}

func unlockFutex(state interrupt.State) {
	futexLock.Unlock()
	interrupt.Restore(state)
}

// Use a single spinlock for atomics. This works fine, since atomics are very
// short sequences of instructions.
func lockAtomics() interrupt.State {
	mask := interrupt.Disable()
	atomicsLock.Lock()
	return mask
}

func unlockAtomics(mask interrupt.State) {
	atomicsLock.Unlock()
	interrupt.Restore(mask)
}

var systemStack [numCPU]uintptr

// Implementation detail of the internal/task package.
// It needs to store the system stack pointer somewhere, and needs to know how
// many cores there are to do so. But it doesn't know the number of cores. Hence
// why this is implemented in the runtime.
func systemStackPtr() *uintptr {
	return &systemStack[currentCPU()]
}

// Color the 'print' and 'println' output according to the current CPU.
// This may be helpful for debugging, but should be disabled otherwise.
const cpuColoredPrint = false

func printlock() {
	// Don't lock the print output inside an interrupt.
	// Locking the print output inside an interrupt can lead to a deadlock: if
	// the interrupt happens while the print lock is held, the interrupt won't
	// be able to take this lock anymore.
	// This isn't great, but the alternative would be to disable interrupts
	// while printing which seems like a worse idea to me.
	if !interrupt.In() {
		printLock.Lock()
	}

	if cpuColoredPrint {
		switch currentCPU() {
		case 1:
			printstring("\x1b[32m") // green
		case 2:
			printstring("\x1b[33m") // yellow
		case 3:
			printstring("\x1b[34m") // blue
		}
	}
}

func printunlock() {
	if cpuColoredPrint {
		if currentCPU() != 0 {
			printstring("\x1b[0m") // reset colored output
		}
	}

	if !interrupt.In() {
		printLock.Unlock()
	}
}
