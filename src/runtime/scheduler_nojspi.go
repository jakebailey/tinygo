//go:build !scheduler.jspi

package runtime

const jspiScheduler = false

func scheduleScheduler(d timeUnit) {
}
