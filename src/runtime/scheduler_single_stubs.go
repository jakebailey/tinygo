//go:build exec.single || exec.asyncify

package runtime

// The scheduler only calls these with more than one core or thread. Targets
// with more than one core define them for multicore execution.

func hasSleepingCore() bool { return false }

func sleepTicksMulticore(d timeUnit) {}

func interruptSleepTicksMulticore(wakeup timeUnit) {}

func schedulerUnlockAndWait() {}

func schedulerWake() {}

func schedulerWakeCore(uint32) {}
