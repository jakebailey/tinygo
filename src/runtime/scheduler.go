package runtime

import "internal/task"

const schedulerDebug = false

var (
	timerQueue         *timerNode
	timerQueueTail     *timerNode
	timerQueueRoot     *timerNode
	timerQueueSequence uint64
)

// Simple logging, for debugging.
func scheduleLog(msg string) {
	if schedulerDebug {
		println("---", msg)
	}
}

// Simple logging with a task pointer, for debugging.
func scheduleLogTask(msg string, t *task.Task) {
	if schedulerDebug {
		println("---", msg, t)
	}
}

// Simple logging with a channel and task pointer.
func scheduleLogChan(msg string, ch *channel, t *task.Task) {
	if schedulerDebug {
		println("---", msg, ch, t)
	}
}

func timerQueueAdd(tn *timerNode) {
	timerQueueSequence++
	tn.queueSequence = timerQueueSequence
	tn.queuePriority = timerQueuePriority(timerQueueSequence)

	if timerQueueRoot == nil {
		timerQueue = tn
		timerQueueTail = tn
		timerQueueRoot = tn
		tn.timer.node = tn
		return
	}

	node := timerQueueRoot
	for {
		if timerQueueLess(tn, node) {
			if node.treeLeft != nil {
				node = node.treeLeft
				continue
			}
			node.treeLeft = tn
			tn.treeParent = node
			tn.next = node
			tn.previous = node.previous
			if node.previous == nil {
				timerQueue = tn
			} else {
				node.previous.next = tn
			}
			node.previous = tn
		} else {
			if node.treeRight != nil {
				node = node.treeRight
				continue
			}
			node.treeRight = tn
			tn.treeParent = node
			tn.previous = node
			tn.next = node.next
			if node.next == nil {
				timerQueueTail = tn
			} else {
				node.next.previous = tn
			}
			node.next = tn
		}
		break
	}

	for tn.treeParent != nil && tn.queuePriority < tn.treeParent.queuePriority {
		if tn == tn.treeParent.treeLeft {
			timerQueueRotateRight(tn.treeParent)
		} else {
			timerQueueRotateLeft(tn.treeParent)
		}
	}
	tn.timer.node = tn
}

func timerQueuePop() *timerNode {
	tn := timerQueue
	timerQueueRemoveNode(tn)
	return tn
}

func timerQueueRemove(t *timer) *timerNode {
	n := t.node
	if n == nil {
		scheduleLog("did not remove timer")
		return nil
	}
	scheduleLog("removed timer")
	timerQueueRemoveNode(n)
	return n
}

func timerQueueRemoveNode(n *timerNode) {
	replacement := timerQueueMerge(n.treeLeft, n.treeRight)
	if replacement != nil {
		replacement.treeParent = n.treeParent
	}
	if n.treeParent == nil {
		timerQueueRoot = replacement
	} else if n == n.treeParent.treeLeft {
		n.treeParent.treeLeft = replacement
	} else {
		n.treeParent.treeRight = replacement
	}

	if n.previous == nil {
		timerQueue = n.next
	} else {
		n.previous.next = n.next
	}
	if n.next == nil {
		timerQueueTail = n.previous
	} else {
		n.next.previous = n.previous
	}
	n.next = nil
	n.previous = nil
	n.treeLeft = nil
	n.treeRight = nil
	n.treeParent = nil
	n.timer.node = nil
}

func timerQueueMerge(left, right *timerNode) *timerNode {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if left.queuePriority < right.queuePriority {
		left.treeRight = timerQueueMerge(left.treeRight, right)
		left.treeRight.treeParent = left
		return left
	}
	right.treeLeft = timerQueueMerge(left, right.treeLeft)
	right.treeLeft.treeParent = right
	return right
}

func timerQueueRotateLeft(node *timerNode) {
	child := node.treeRight
	node.treeRight = child.treeLeft
	if node.treeRight != nil {
		node.treeRight.treeParent = node
	}
	timerQueueReplaceTreeNode(node, child)
	child.treeLeft = node
	node.treeParent = child
}

func timerQueueRotateRight(node *timerNode) {
	child := node.treeLeft
	node.treeLeft = child.treeRight
	if node.treeLeft != nil {
		node.treeLeft.treeParent = node
	}
	timerQueueReplaceTreeNode(node, child)
	child.treeRight = node
	node.treeParent = child
}

func timerQueueReplaceTreeNode(node, replacement *timerNode) {
	replacement.treeParent = node.treeParent
	if node.treeParent == nil {
		timerQueueRoot = replacement
	} else if node == node.treeParent.treeLeft {
		node.treeParent.treeLeft = replacement
	} else {
		node.treeParent.treeRight = replacement
	}
}

func timerQueueLess(left, right *timerNode) bool {
	leftWhen := left.whenTicks()
	rightWhen := right.whenTicks()
	if leftWhen != rightWhen {
		return leftWhen < rightWhen
	}
	return left.queueSequence < right.queueSequence
}

func timerQueuePriority(sequence uint64) uint64 {
	value := sequence + 0x9e3779b97f4a7c15
	value = (value ^ value>>30) * 0xbf58476d1ce4e5b9
	value = (value ^ value>>27) * 0x94d049bb133111eb
	return value ^ value>>31
}

// firingTimers is a list of timer nodes whose callback is currently running.
// It is only used by schedulers that run timer callbacks concurrently with user
// goroutines (the threads and cores schedulers), so that a timer stopped or
// reset while its callback is running is not re-added to the timer queue by a
// periodic timer's callback. Access is protected by the scheduler's timer lock.
var firingTimers *timerNode

// firingTimersAdd marks the given timer node as currently firing. The caller
// must hold the scheduler's timer lock.
func firingTimersAdd(tn *timerNode) {
	tn.stopped = false
	tn.firingNext = firingTimers
	firingTimers = tn
}

// firingTimersRemove removes the given timer node from the firing list. The
// caller must hold the scheduler's timer lock.
func firingTimersRemove(tn *timerNode) {
	for q := &firingTimers; *q != nil; q = &(*q).firingNext {
		if *q == tn {
			*q = tn.firingNext
			tn.firingNext = nil
			return
		}
	}
}

// firingTimerStop marks a currently-firing timer as stopped, so that its
// callback will not re-add it to the queue. It returns whether the timer is
// currently firing. The caller must hold the scheduler's timer lock.
func firingTimerStop(tim *timer) bool {
	for tn := firingTimers; tn != nil; tn = tn.firingNext {
		if tn.timer == tim {
			tn.stopped = true
			return true
		}
	}
	return false
}

// Goexit terminates the currently running goroutine. No other goroutines are affected.
func Goexit() {
	panicOrGoexit(nil, panicGoexit)
}

//go:linkname fips_getIndicator crypto/internal/fips140.getIndicator
func fips_getIndicator() uint8 {
	return task.Current().FipsIndicator
}

//go:linkname fips_setIndicator crypto/internal/fips140.setIndicator
func fips_setIndicator(indicator uint8) {
	// This indicator is stored per goroutine.
	task.Current().FipsIndicator = indicator
}

//go:linkname fips140_setBypass crypto/fips140.setBypass
func fips140_setBypass() {
	task.Current().FipsOnlyBypass = true
}

//go:linkname fips140_unsetBypass crypto/fips140.unsetBypass
func fips140_unsetBypass() {
	task.Current().FipsOnlyBypass = false
}

//go:linkname fips140_isBypassed crypto/fips140.isBypassed
func fips140_isBypassed() bool {
	return task.Current().FipsOnlyBypass
}
