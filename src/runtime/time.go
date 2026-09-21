package runtime

import (
	"unsafe"
)

// This is the timer that's used internally inside the runtime.
type timer struct {
	lock timerLock

	// When to call the timer, and the interval for the ticker.
	when   int64
	period int64

	// Callback from the time package.
	f   func(arg any, seq uintptr, delta int64)
	arg any

	synctest *synctestBubble
	node     *timerNode
	// Keep a full channel ticker out of the queue. Without weak pointers, the
	// queue would retain and continuously run an otherwise unreachable ticker.
	pausedNode         *timerNode
	c                  unsafe.Pointer
	isChan             bool
	stopped            bool
	suppressedCallback bool
}

func (tim *timer) callCallback(delta int64) {
	tim.f(tim.arg, 0, delta)
}

func (tim *timer) nextWhen(delta int64) int64 {
	next := tim.when + tim.period*(1+delta/tim.period)
	if next < 0 {
		return 1<<63 - 1
	}
	return next
}

// This is the struct used internally in the runtime. The first two fields are
// the same as time.Timer and time.Ticker so it can be used as-is in the time
// package.
type timeTimer struct {
	c         unsafe.Pointer // <-chan time.Time
	initTimer bool
	timer
}

//go:linkname newTimer time.newTimer
func newTimer(when, period int64, f func(arg any, seq uintptr, delta int64), arg any, c unsafe.Pointer) *timeTimer {
	bubble := currentSynctestBubble()
	tim := &timeTimer{
		c:         c,
		initTimer: true,
		timer: timer{
			when:     when,
			period:   period,
			f:        f,
			arg:      arg,
			synctest: bubble,
			c:        c,
			isChan:   c != nil,
		},
	}
	if c != nil {
		(*channel)(c).timer = &tim.timer
	}
	scheduleLog("new timer")
	node := &timerNode{
		timer:    &tim.timer,
		callback: timerCallback,
	}
	if bubble != nil {
		bubble.addTimer(node)
	} else {
		addTimer(node)
	}
	return tim
}

//go:linkname stopTimer time.stopTimer
func stopTimer(tim *timeTimer) bool {
	if tim.timer.synctest != nil {
		tim.timer.synctest.checkTimerAccess("stop")
	}
	tim.timer.lock.Lock()
	tim.timer.suppressedCallback = false
	tim.timer.stopped = true
	var removed bool
	if tim.timer.synctest != nil {
		removed = tim.timer.synctest.removeTimer(&tim.timer) != nil
	} else {
		removed = removeTimer(&tim.timer) != nil
	}
	if tim.timer.pausedNode != nil {
		tim.timer.pausedNode = nil
		removed = true
	}
	drained := timerChanDrain(tim.c)
	suppressed := tim.timer.suppressedCallback
	tim.timer.lock.Unlock()
	return removed || drained || suppressed
}

//go:linkname resetTimer time.resetTimer
func resetTimer(t *timeTimer, when, period int64) bool {
	if t.timer.synctest != nil {
		t.timer.synctest.checkTimerAccess("reset")
	}
	t.timer.lock.Lock()
	t.timer.suppressedCallback = false
	var n *timerNode
	if t.timer.synctest != nil {
		n = t.timer.synctest.removeTimer(&t.timer)
	} else {
		n = removeTimer(&t.timer)
	}
	if n == nil && t.timer.pausedNode != nil {
		n = t.timer.pausedNode
		t.timer.pausedNode = nil
	}
	removed := n != nil
	drained := timerChanDrain(t.c)
	suppressed := t.timer.suppressedCallback
	if n == nil {
		// Allocation can start GC, so do not hold the cores spin lock.
		t.timer.lock.Unlock()
		replacement := new(timerNode)
		t.timer.lock.Lock()
		// A concurrent reset can queue the timer during allocation.
		// Remove it again so this reset takes effect after that operation.
		if t.timer.synctest != nil {
			n = t.timer.synctest.removeTimer(&t.timer)
		} else {
			n = removeTimer(&t.timer)
		}
		removed = n != nil
		if n == nil {
			n = replacement
		}
	}
	t.timer.when = when
	t.timer.period = period
	t.timer.stopped = false
	n.timer = &t.timer
	n.callback = timerCallback
	var runNow bool
	if t.timer.synctest != nil {
		runNow = t.timer.synctest.queueTimer(n)
	} else {
		addTimer(n)
	}
	t.timer.lock.Unlock()
	if runNow {
		n.callback(n, 0)
	}
	return removed || drained || suppressed
}

func timerChanRearm(tim *timer) {
	if tim == nil {
		return
	}
	tim.lock.Lock()
	tn := tim.pausedNode
	if tn == nil || tim.stopped {
		tim.lock.Unlock()
		return
	}
	tim.pausedNode = nil
	if now := nanotime(); now > tim.when {
		tim.when = tim.nextWhen(now - tim.when)
	}
	addTimer(tn)
	tim.lock.Unlock()
}

//go:linkname time_runtimeNano time.runtimeNano
func time_runtimeNano() int64 {
	if bubble := currentSynctestBubble(); bubble != nil {
		return bubble.time()
	}
	return nanotime()
}

//go:linkname time_runtimeNow time.runtimeNow
func time_runtimeNow() (sec int64, nsec int32, mono int64) {
	if bubble := currentSynctestBubble(); bubble != nil {
		now := bubble.time()
		return now / 1e9, int32(now % 1e9), 0
	}
	return now()
}

// timerNode is an element in the timer queue's treap and ordered list.
type timerNode struct {
	next          *timerNode
	previous      *timerNode
	treeLeft      *timerNode
	treeRight     *timerNode
	treeParent    *timerNode
	queueSequence uint64
	queuePriority uint64
	timer         *timer
	callback      func(node *timerNode, delta int64)

	// The following fields are only used by schedulers that run timer
	// callbacks concurrently with user goroutines (the threads and cores
	// schedulers). They make it possible to stop or reset a periodic timer (a
	// ticker) while its callback is running, without the callback re-adding the
	// timer to the queue afterwards. They are protected by the scheduler's
	// timer lock for normal timers and the bubble lock for synctest timers.
	//
	// firingNext links nodes whose callback is currently running into the
	// firingTimers list. stopped is set when the timer was stopped or reset
	// while its callback was running, so that timerCallback does not re-add it.
	firingNext *timerNode
	stopped    bool
}

// whenTicks returns the (absolute) time when this timer should trigger next.
func (t *timerNode) whenTicks() timeUnit {
	return nanosecondsToTicks(t.timer.when)
}

// timerCallback is called when a timer expires. It makes sure to call the
// callback in the time package and to re-add the timer to the queue if this is
// a ticker (repeating timer).
// This is intentionally used as a callback and not a direct call (even though a
// direct call would be trivial), because otherwise a circular dependency
// between scheduler, addTimer and timerQueue would form. Such a circular
// dependency causes timerQueue not to get optimized away.
// If timerQueue doesn't get optimized away, small programs (that don't call
// time.NewTimer etc) would still pay the cost of these timers.
func timerCallback(tn *timerNode, delta int64) {
	tn.timer.lock.Lock()

	// Run timer function (implemented in the time package).
	// The seq parameter to the f function is not used in the time
	// package so is left zero.
	if !tn.stopped || !tn.timer.isChan {
		tn.timer.callCallback(delta)
	}

	// Finish firing the timer and re-add it if it is periodic.
	if tn.timer.synctest != nil {
		tn.timer.synctest.finishTimer(tn)
	} else {
		reAddTimer(tn, delta)
	}
	tn.timer.lock.Unlock()
}

//go:linkname time_runtimeIsBubbled time.runtimeIsBubbled
func time_runtimeIsBubbled() bool {
	return currentSynctestBubble() != nil
}
