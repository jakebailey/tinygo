//go:build scheduler.unified && (exec.single || exec.asyncify || exec.multicore || exec.workers)

package runtime

// This file implements the TinyGo scheduler. Goroutines run until they block or
// yield, then the scheduler picks the next one from a FIFO run queue. Sleeping
// goroutines wait in a sleep queue ordered by wakeup time, next to the timer
// queue.
//
// The same scheduler runs on one thread, on microcontroller cores, and on a
// pool of OS threads. A backend provides the scheduler lock and the
// functions that pick, wake and wait for work:
//   - scheduler_single.go runs everything on one thread. Its scheduler lock
//     does nothing and the multicore parts compile away.
//   - scheduler_multicore.go runs goroutines on all active microcontroller cores.
//   - scheduler_workers.go runs goroutines on a pool of OS threads.

import "internal/task"

const hasScheduler = true

var (
	sleepQueue      *task.Task
	runqueue        task.Queue
	deadlockedTasks task.Queue
)

// Set to true when the scheduler should exit after the next switch to the
// scheduler. This is a special case for //go:wasmexport.
var schedulerExit bool

// finalizerIdleGC runs pressure GC at safe points and enables asyncify stack cleanup.
// The first finalizer installs it so unused code can be removed.
var finalizerIdleGC func() bool

// deadlock is called when a goroutine cannot proceed any more, but is in theory
// not exited (so deferred calls won't run). This can happen for example in code
// like this, that blocks forever:
//
//	select{}
//
//go:noinline
func deadlock() {
	// Keep permanently blocked tasks reachable so their suspended stacks remain
	// GC roots, but never put them back on the runnable queue.
	current := task.Current()
	synctestTaskBlock(current)
	deadlockedTasks.Push(current)
	task.Pause()
	runtimeFatal("unreachable")
}

// exitGoroutine ends an asyncify task that returned from its function.
// Unlike deadlock, this task will not resume.
func exitGoroutine() {
	task.Exit()
	runtimeFatal("unreachable")
}

// wasmExportExit stops the scheduler after a //go:wasmexport function returns.
// It is not used when the scheduler is disabled.
func wasmExportExit() {
	schedulerExit = true
	if finalizerIdleGC != nil {
		task.MarkFinishing()
	}

	task.Pause()

	// TODO: we could cache the allocated stack so we don't have to keep
	// allocating a new stack on every //go:wasmexport call.
}

func goexit() {
	task.Goexit()
}

// Mark the given task as ready to resume.
// This is allowed even if the task isn't paused yet, but will pause soon.
func scheduleTask(t *task.Task) {
	synctestTaskWake(t)
	scheduleTaskNoWake(t)
}

func scheduleTaskNoWake(t *task.Task) {
	if !hasParallelism {
		// The task can't be running on another thread, and interrupt handlers
		// call this function so it must not take the scheduler lock.
		runqueue.Push(t)
		return
	}
	lockScheduler()
	switch t.RunState {
	case task.RunStatePaused:
		// Paused, state is saved on the stack.
		// Add it to the runqueue...
		schedulerQueue(t)
		// ...and wake up a sleeping core, if there is one.
		// (If all cores are already busy, this is a no-op).
		schedulerWakeTask(t)
	case task.RunStateRunning:
		// Not yet paused (probably going to pause very soon), so let the
		// Pause() function know it can resume immediately.
		t.RunState = task.RunStateResuming
	default:
		if schedulerAsserts {
			runtimeFatal("scheduler: unknown run state")
		}
	}
	unlockScheduler()
}

func addSleepTask(t *task.Task, wakeup timeUnit) {
	// Save the timestamp when the task should be woken up.
	t.Data = uint64(wakeup)

	// If another core is currently using the timer, make sure it wakes up at
	// the right time.
	if hasParallelism {
		interruptSleepTicksMulticore(wakeup)
	}

	// Find the position where we should insert this task in the queue.
	q := &sleepQueue
	for {
		if *q == nil {
			// Found the end of the time queue. Insert it here, at the end.
			break
		}
		if timeUnit((*q).Data) > timeUnit(t.Data) {
			// Found a task in the queue that has a timeout before the
			// to-be-sleeping task. Insert our task right before.
			break
		}
		q = &(*q).Next
	}

	// Insert the task into the queue (this could be at the end, if *q is nil).
	t.Next = *q
	*q = t
}

func Gosched() {
	lockScheduler()
	schedulerQueue(task.Current())
	task.PauseLocked()
}

// reserveTimerRoom makes sure the timer queue can take one more node, plus one
// re-added node for each core running a timer callback, without allocating.
// Allocating with the lock held can start a GC that waits for a core spinning on
// the lock with interrupts off. The lock is released while allocating. Call with
// the scheduler lock held.
func reserveTimerRoom() {
	for {
		need := timerQueueRoom(maxSchedulerThreads)
		if !timerQueueFull(need) {
			return
		}
		n := 2*need + 1
		if timerQueue != nil && 2*cap(timerQueue.nodes)+1 > n {
			n = 2*cap(timerQueue.nodes) + 1
		}
		unlockScheduler()
		h := &timerHeap{nodes: make([]*timerNode, 0, n)}
		lockScheduler()
		timerQueueGrow(h)
	}
}

func addTimer(tn *timerNode) {
	mask := lockTimers()
	if hasParallelism {
		reserveTimerRoom()
	}
	timerQueueAdd(tn)
	if hasParallelism {
		interruptSleepTicksMulticore(tn.whenTicks())
	}
	unlockTimers(mask)
}

// reAddTimer finishes firing a timer. It re-adds periodic timers unless they
// were stopped or reset while the callback was running.
func reAddTimer(tn *timerNode, delta int64) {
	mask := lockTimers()

	if hasParallelism {
		// Remove the timer from the firing list before re-adding it to the
		// queue, so that another core popping it off the queue can't insert it
		// into the firing list a second time (which would corrupt the list).
		firingTimersRemove(tn)
	}

	// A timer stopped or reset while its callback was running must not be
	// re-added: a stopped ticker must stay stopped, and a reset ticker has
	// already been re-added by resetTimer.
	if tn.stopped || tn.timer.stopped || tn.timer.period == 0 {
		freeTimerNode(tn)
		unlockTimers(mask)
		return
	}
	tn.timer.when = tn.timer.nextWhen(delta)
	if tn.timer.isChan && tn.timer.blocked == 0 {
		tn.timer.pausedNode = tn
		unlockTimers(mask)
		return
	}
	// addTimer reserved room for this node while the callback runs.
	timerQueueAdd(tn)
	if hasParallelism {
		interruptSleepTicksMulticore(tn.whenTicks())
	}
	unlockTimers(mask)
}

func removeTimer(t *timer, stopFiring bool) *timerNode {
	mask := lockTimers()
	n := timerQueueRemove(t)
	if hasParallelism && n == nil && stopFiring {
		// The timer wasn't in the queue. It might be running its callback right
		// now; if so, mark it stopped so it won't be re-added.
		firingTimerStop(t)
	}
	unlockTimers(mask)
	return n
}

func schedulerRunQueue() *task.Queue {
	return &runqueue
}

// Pause the current task for a given time.
//
//go:linkname sleep time.Sleep
func sleep(duration int64) {
	if duration <= 0 {
		return
	}
	if synctestSleep(duration) {
		return
	}

	delay := nanosecondsToTicks(duration)
	now := ticks()
	maxTime := timeUnit(1<<63 - 1)
	wakeup := maxTime
	if delay <= 0 || now <= maxTime-delay {
		wakeup = now + delay
	}

	// While the scheduler is locked:
	// - add this task to the sleep queue
	// - switch to the scheduler (only allowed while locked)
	// - let the scheduler handle it from there
	lockScheduler()
	addSleepTask(task.Current(), wakeup)
	task.PauseLocked()
}

// Run the scheduler until main returns.
// There are a few special cases:
//   - When returnAtDeadlock is true, it also returns when there are no more
//     runnable goroutines.
//   - When using asyncify stack switching, it returns when it has to wait
//     (JavaScript uses setTimeout so the scheduler must return to the JS
//     environment).
func scheduler(returnAtDeadlock bool) {
	lockScheduler()
	schedule(returnAtDeadlock)
	unlockScheduler()
}

// The scheduler loop. Call with the scheduler lock held, it is held again on
// return.
func schedule(returnAtDeadlock bool) {
	for !schedulerMainExited() {
		schedulerAcquireSlot()

		var now timeUnit
		if sleepQueue != nil || timerQueuePeek() != nil {
			now = ticks()

			// Check whether the first task in the sleep queue is ready to run.
			if sleepingTask := sleepQueue; sleepingTask != nil && now >= timeUnit(sleepingTask.Data) {
				// It is, pop it from the queue.
				sleepQueue = sleepQueue.Next
				sleepingTask.Next = nil

				if !hasParallelism {
					// Run it after the goroutines that are already runnable.
					runqueue.Push(sleepingTask)
				} else if !schedulerCanRun(sleepingTask) {
					// Assigned to another core, so hand it over.
					schedulerQueue(sleepingTask)
					schedulerWakeTask(sleepingTask)
					continue
				} else {
					// Run it now.
					schedulerRunTask(sleepingTask)
					continue
				}
			}

			// Check whether a timer has expired that needs to be run.
			if next := timerQueuePeek(); next != nil && now >= next.whenTicks() {
				delay := ticksToNanoseconds(now - next.whenTicks())
				// Pop timer from queue.
				tn := timerQueuePop()

				if hasParallelism {
					// Mark the timer as firing, so that a concurrent Stop or
					// Reset (via removeTimer) can prevent a periodic timer from
					// re-adding itself in its callback.
					firingTimersAdd(tn)
				}

				// Run the callback stored in this timer node.
				schedulerHandOffClock()
				unlockScheduler()
				tn.callback(tn, delay)
				lockScheduler()
				if hasParallelism {
					continue
				}
			}
		}

		// Check for ready-to-run tasks.
		if runnable := schedulerPop(); runnable != nil {
			schedulerRunTask(runnable)

			// The last call to Resume() was a signal to stop the scheduler
			// since a //go:wasmexport function returned.
			if GOARCH == "wasm" && schedulerExit {
				schedulerExit = false // reset the signal
				if task.Current() == nil {
					if finalizerIdleGC != nil {
						finalizerIdleGC()
					}
					// Return from an export at the top level before unrelated
					// goroutines run. A nested export returns to its active
					// outer scheduler.
					if asyncScheduler && (!runqueue.Empty() || sleepQueue != nil || timerQueuePeek() != nil) {
						sleepTicks(0)
					}
				}
				return
			}
			continue
		}

		// At this point, there are no runnable tasks anymore.

		// Run the pressure GC only when the scheduler is idle at the top level.
		// This batches completed work and avoids collections during allocation.
		if finalizerIdleGC != nil && task.Current() == nil {
			unlockScheduler()
			collected := finalizerIdleGC()
			lockScheduler()
			if collected {
				continue
			}
		}

		// If another core is using the clock, let it handle the sleep queue.
		if hasParallelism && hasSleepingCore() {
			schedulerIdle()
			continue
		}

		if sleepQueue == nil && timerQueuePeek() == nil {
			// No runnable tasks and no sleeping tasks or timers. There's
			// nothing to do.
			if returnAtDeadlock || asyncScheduler {
				// JavaScript is treated specially, see below.
				return
			}
			// Wait until something happens (like an interrupt).
			schedulerIdle()
			continue
		}

		// The timer is free to use, so check whether there are any future
		// tasks/timers that we can wait for.
		var timeLeft timeUnit
		if sleepingTask := sleepQueue; sleepingTask != nil {
			// We already checked that there is no ready-to-run sleeping task
			// (using the same 'now' value), so timeLeft will always be
			// positive.
			timeLeft = timeUnit(sleepingTask.Data) - now
		}
		if next := timerQueuePeek(); next != nil {
			// If the timer queue needs to run earlier, reduce the time we are
			// going to sleep.
			// Like with sleepQueue, we already know there is no timer ready to
			// run since we already checked above.
			timeLeftForTimer := next.whenTicks() - now
			if sleepQueue == nil || timeLeftForTimer < timeLeft {
				timeLeft = timeLeftForTimer
			}
		}

		if schedulerDebug {
			println("  sleeping...", sleepQueue, uint(timeLeft))
			for t := sleepQueue; t != nil; t = t.Next {
				println("    task sleeping:", t, timeUnit(t.Data))
			}
			if timerQueue != nil {
				for _, tim := range timerQueue.nodes {
					println("---   timer waiting:", tim, tim.whenTicks())
				}
			}
		}
		if timeLeft > 0 {
			// Sleep for a bit until the next task or timer is ready to run.
			schedulerSleep(timeLeft)
			if asyncScheduler {
				// The sleepTicks function above only sets a timeout at which
				// point the scheduler will be called again. It does not really
				// sleep. So instead of sleeping, we return and expect to be
				// called again.
				return
			}
		}
	}
}

// Run the given task until it pauses. Call with the scheduler lock held, it is
// held again on return.
func schedulerRunTask(t *task.Task) {
	scheduleLogTask("  run:", t)
	schedulerHandOffClock()
	setCurrentTask(t)
	if hasParallelism {
		t.RunState = task.RunStateRunning
	}
	unlockScheduler() // unlock before resuming, Pause() will lock again
	t.Resume()
	setCurrentTask(nil)
}

// This core stops watching the clock while it runs a task, so wake a waiting core
// to watch it if tasks sleep or timers wait. Call with the scheduler lock held.
func schedulerHandOffClock() {
	if hasParallelism && (sleepQueue != nil || timerQueue != nil) && !hasSleepingCore() {
		schedulerWake()
	}
}

// Sleep until the given time has passed, or until something wakes up this core.
// Call with the scheduler lock held, it is held again on return.
func schedulerSleep(d timeUnit) {
	if hasParallelism {
		sleepTicksMulticore(d)
		return
	}
	// Interrupts may wake up a goroutine while sleeping.
	unlockScheduler()
	sleepTicks(d)
	lockScheduler()
}

// Wait until there is work to do. Call with the scheduler lock held, it is held
// again on return.
func schedulerIdle() {
	if hasParallelism {
		schedulerUnlockAndWait()
		return
	}
	unlockScheduler()
	waitForEvents()
	lockScheduler()
}
