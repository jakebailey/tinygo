//go:build scheduler.cores

package runtime

// Scheduler shared by all schedulers that run goroutines on more than one
// core or thread at a time. Each of them provides the per-thread state and
// the functions to pick, wake and wait for work.

import (
	"internal/task"
	"runtime/interrupt"
	"sync/atomic"
)

const hasScheduler = true

const hasParallelism = true

var mainExited atomic.Uint32

var (
	sleepQueue *task.Task
	runqueue   task.Queue
)

func deadlock() {
	// Call yield without requesting a wakeup.
	synctestTaskBlock(task.Current())
	task.Pause()
	trap()
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
	schedulerLock.Lock()
	switch t.RunState {
	case task.RunStatePaused:
		// Paused, state is saved on the stack.
		// Add it to the runqueue...
		runqueue.Push(t)
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
	schedulerLock.Unlock()
}

func addSleepTask(t *task.Task, wakeup timeUnit) {
	// Save the timestamp when the task should be woken up.
	t.Data = uint64(wakeup)

	// If another core is currently using the timer, make sure it wakes up at
	// the right time.
	interruptSleepTicksMulticore(wakeup)

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
	schedulerLock.Lock()
	runqueue.Push(task.Current())
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
		schedulerLock.Unlock()
		h := &timerHeap{nodes: make([]*timerNode, 0, n)}
		schedulerLock.Lock()
		timerQueueGrow(h)
	}
}

func addTimer(tn *timerNode) {
	schedulerLock.Lock()
	reserveTimerRoom()
	timerQueueAdd(tn)
	interruptSleepTicksMulticore(tn.whenTicks())
	schedulerLock.Unlock()
}

// reAddTimer finishes firing a timer. It re-adds periodic timers unless they
// were stopped or reset while the callback was running.
func reAddTimer(tn *timerNode, delta int64) {
	schedulerLock.Lock()

	// Remove the timer from the firing list before re-adding it to the queue,
	// so that another core popping it off the queue can't insert it into the
	// firing list a second time (which would corrupt the list).
	firingTimersRemove(tn)

	if tn.stopped || tn.timer.stopped {
		// The timer was stopped or reset while its callback was running. Don't
		// re-add it: a stopped ticker must stay stopped, and a reset ticker has
		// already been re-added by resetTimer.
		freeTimerNode(tn)
		schedulerLock.Unlock()
		return
	}
	if tn.timer.period == 0 {
		freeTimerNode(tn)
		schedulerLock.Unlock()
		return
	}
	tn.timer.when = tn.timer.nextWhen(delta)
	if tn.timer.isChan && tn.timer.blocked == 0 {
		tn.timer.pausedNode = tn
		schedulerLock.Unlock()
		return
	}
	timerQueueAdd(tn)
	interruptSleepTicksMulticore(tn.whenTicks())
	schedulerLock.Unlock()
}

func removeTimer(t *timer, stopFiring bool) *timerNode {
	schedulerLock.Lock()
	n := timerQueueRemove(t)
	if n == nil && stopFiring {
		// The timer wasn't in the queue. It might be running its callback right
		// now; if so, mark it stopped so it won't be re-added.
		firingTimerStop(t)
	}
	schedulerLock.Unlock()
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
	schedulerLock.Lock()
	addSleepTask(task.Current(), wakeup)
	task.PauseLocked()
}

func scheduler(_ bool) {
	for mainExited.Load() == 0 {
		var now timeUnit
		if sleepQueue != nil || timerQueuePeek() != nil {
			now = ticks()

			// Check whether the first task in the sleep queue is ready to run.
			if sleepingTask := sleepQueue; sleepingTask != nil && now >= timeUnit(sleepingTask.Data) {
				// It is, pop it from the queue.
				sleepQueue = sleepQueue.Next
				sleepingTask.Next = nil

				if !schedulerCanRun(sleepingTask) {
					// Assigned to another core, so hand it over.
					runqueue.Push(sleepingTask)
					schedulerWakeTask(sleepingTask)
					continue
				}

				// Run it now.
				schedulerHandOffClock()
				setCurrentTask(sleepingTask)
				sleepingTask.RunState = task.RunStateRunning
				schedulerLock.Unlock() // unlock before resuming, Pause() will lock again
				sleepingTask.Resume()
				setCurrentTask(nil)
				continue
			}

			// Check whether a timer has expired that needs to be run.
			if next := timerQueuePeek(); next != nil && now >= next.whenTicks() {
				delay := ticksToNanoseconds(now - next.whenTicks())
				// Pop timer from queue.
				tn := timerQueuePop()

				// Mark the timer as firing, so that a concurrent Stop or Reset
				// (via removeTimer) can prevent a periodic timer from re-adding
				// itself in its callback.
				firingTimersAdd(tn)

				// Run the callback stored in this timer node.
				schedulerHandOffClock()
				schedulerLock.Unlock()
				tn.callback(tn, delay)
				schedulerLock.Lock()
				continue
			}
		}

		// Check for ready-to-run tasks.
		if runnable := schedulerPop(); runnable != nil {
			// Resume it now.
			schedulerHandOffClock()
			setCurrentTask(runnable)
			runnable.RunState = task.RunStateRunning
			schedulerLock.Unlock() // unlock before resuming, Pause() will lock again
			runnable.Resume()
			setCurrentTask(nil)

			continue
		}

		// At this point, there are no runnable tasks anymore.
		// If another core is using the clock, let it handle the sleep queue.
		if hasSleepingCore() {
			schedulerUnlockAndWait()
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

		if timeLeft > 0 {
			// Sleep for a bit until the next task or timer is ready to run.
			sleepTicksMulticore(timeLeft)
			continue
		}

		// No runnable tasks and no sleeping tasks or timers. There's nothing to
		// do.
		// Wait until something happens (like an interrupt).
		schedulerUnlockAndWait()
	}
}

// This core stops watching the clock while it runs a task, so wake a waiting core
// to watch it if tasks sleep or timers wait. Call with the scheduler lock held.
func schedulerHandOffClock() {
	if (sleepQueue != nil || timerQueue != nil) && !hasSleepingCore() {
		schedulerWake()
	}
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
