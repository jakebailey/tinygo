//go:build scheduler.unified && (exec.single || exec.asyncify)

package runtime

// Backend of the scheduler that runs all goroutines on a single thread.

import (
	"internal/task"
	"runtime/interrupt"
)

// On JavaScript, we can't do a blocking sleep. Instead we have to return and
// queue a new scheduler invocation using setTimeout.
const asyncScheduler = GOOS == "js"

// Concurrency is not parallelism. While this scheduler has concurrency, it does
// not have parallelism.
const hasParallelism = false

// Only one thread runs the scheduler.
const maxSchedulerThreads = 1

// Set to true after main.main returns.
var mainExited bool

func schedulerMainExited() bool {
	return mainExited
}

// Interrupt handlers only touch the run queue, which has its own lock, so the
// scheduler lock does nothing.
func lockScheduler() {}

func unlockScheduler() {}

// Interrupt handlers may change timers, so keep interrupts off while using the
// timer queue.
func lockTimers() interrupt.State {
	return interrupt.Disable()
}

func unlockTimers(mask interrupt.State) {
	interrupt.Restore(mask)
}

// run is called by the program entry point to execute the go program.
// With a scheduler, init and the main function are invoked in a goroutine before starting the scheduler.
func run() {
	initRand()
	initHeap()
	go func() {
		initAll()
		callMain()
		mainExited = true
	}()
	scheduler(false)
}

// NumCPU returns the number of logical CPUs usable by the current process.
func NumCPU() int {
	return 1
}

// The only thread always runs the scheduler.
func schedulerAcquireSlot() {}

func schedulerQueue(t *task.Task) {
	runqueue.Push(t)
}

func schedulerWakeTask(t *task.Task) {}

func schedulerPop() *task.Task {
	return runqueue.Pop()
}

func schedulerCanRun(t *task.Task) bool {
	return true
}

// The internal/task package keeps track of the current task.
func setCurrentTask(t *task.Task) {}

func lockAtomics() interrupt.State {
	return interrupt.Disable()
}

func unlockAtomics(mask interrupt.State) {
	interrupt.Restore(mask)
}

func printlock() {
	// nothing to do
}

func printunlock() {
	// nothing to do
}
