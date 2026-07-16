//go:build scheduler.tasks || scheduler.asyncify || scheduler.jspi || scheduler.cores

package task

import "sync/atomic"

var (
	mainTask           *Task
	liveTasks          uint32
	mainExitedByGoexit uint32
)

func addLiveTask(t *Task) {
	if mainTask == nil {
		mainTask = t
	}
	atomic.AddUint32(&liveTasks, 1)
}

// Exit exits the current task because runtime.Goexit was called.
func Exit() {
	exit(true)
}

func exit(goexit bool) {
	t := Current()
	finish(t, goexit)
	terminate()
	runtimePanic("unreachable")
}

func finish(t *Task, goexit bool) {
	remaining := atomic.AddUint32(&liveTasks, ^uint32(0))
	if t == mainTask {
		if goexit {
			if remaining == 0 {
				runtimePanic("all goroutines are asleep - deadlock!")
			}
			atomic.StoreUint32(&mainExitedByGoexit, 1)
		}
	} else if atomic.LoadUint32(&mainExitedByGoexit) != 0 && remaining == 0 {
		runtimePanic("all goroutines are asleep - deadlock!")
	}
}
