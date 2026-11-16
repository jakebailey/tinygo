//go:build (scheduler.tasks || scheduler.asyncify) && !rp2040 && !rp2350 && !(tinygo.riscv && virt && qemu)

package runtime

// The scheduler only calls these with more than one core or thread. Targets
// with more than one core define them for the cores scheduler.

func hasSleepingCore() bool { return false }

func sleepTicksMulticore(d timeUnit) {}

func interruptSleepTicksMulticore(wakeup timeUnit) {}

func schedulerUnlockAndWait() {}

func schedulerWake() {}
