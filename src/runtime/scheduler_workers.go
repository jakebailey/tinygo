//go:build scheduler.unified && exec.workers

package runtime

// The workers execution mode runs goroutines on a pool of OS threads.
// At most gomaxprocs workers run goroutines at the same time: each of them
// holds a slot. A worker without a slot is idle, watches the clock, or is
// blocked in a system call.
//
// Goroutines switch stacks cooperatively and share the run queue, sleep queue
// and timers with the other execution modes (scheduler_cooperative.go).
//
// Limitations:
//   - Goroutines are not preempted. A goroutine that runs without blocking or
//     calling runtime.Gosched keeps its slot, and with all slots taken this
//     way other goroutines and timers don't run.
//   - Only system calls made through the syscall package give up the slot
//     while blocked. Blocking cgo calls keep it.
//   - Go code must not run on threads created by C code.
//   - GOMAXPROCS is read once from the environment at startup.

import (
	"internal/futex"
	"internal/task"
	"runtime/interrupt"
	"unsafe"
)

// Stack size of worker threads other than the main thread. Goroutines run on
// their own stacks, this stack is only used by the scheduler and timers.
const workerStackSize = 256 * 1024

type worker struct {
	// Next worker in allWorkers, guarded by workersLock.
	next *worker

	// Next worker in idleWorkers, guarded by schedulerLock.
	idleNext *worker

	// The thread of this worker: a pthread_t, or a handle on Windows.
	thread uintptr

	// Goroutine running on this worker, or nil while in the scheduler.
	task *task.Task

	// Stack pointer of the scheduler while a goroutine runs on this worker.
	systemStack uintptr

	// Bounds of the system stack of this worker.
	stackBottom uintptr
	stackTop    uintptr

	// Nonzero while the worker may continue after parking.
	wake futex.Futex

	// Whether this worker may run goroutines, guarded by schedulerLock.
	hasSlot bool

	// Stack pointer and registers where the worker stopped for the GC.
	gcSP   uintptr
	gcRegs workerGCRegs

	// Goroutine locked to this worker with LockOSThread, guarded by
	// schedulerLock. The worker runs no other goroutine while it is set.
	locked *task.Task

	// Whether the locked goroutine is ready to run, guarded by schedulerLock.
	lockedReady bool

	// Whether this worker waits for its locked goroutine to become ready,
	// guarded by schedulerLock.
	lockedParked bool

	// Whether the locked goroutine has exited. Only used by this worker.
	lockedExited bool

	// LockOSThread calls by the program and by the runtime. Only used by the
	// locked goroutine.
	lockedExt uint32
	lockedInt uint32
}

var (
	schedulerLock hostMutex
	futexLock     hostMutex
	atomicsLock   hostMutex
	printLock     hostMutex

	// All workers, guarded by workersLock. The GC holds workersLock while the
	// world is stopped. Lock order is schedulerLock, then workersLock.
	workersLock hostMutex
	allWorkers  *worker

	// The rest is guarded by schedulerLock.
	idleWorkers   *worker
	clockWorker   *worker
	clockDeadline timeUnit
	slotsUsed     int32
	inSyscall     int32
	mainWorker    *worker

	// Number of workers with a locked goroutine that isn't blocked.
	lockedRunning int32

	numCPU     int32
	gomaxprocs int32

	// Number of workers that may run timer callbacks at the same time.
	maxSchedulerThreads int
)

// hostMutex is a mutex that blocks the OS thread. It is used for short
// critical sections that never pause the current goroutine.
type hostMutex struct {
	futex futex.Futex
}

func (m *hostMutex) Lock() {
	if m.futex.CompareAndSwap(0, 1) {
		return
	}
	for i := 0; i < 100; i++ {
		if m.futex.Load() == 0 && m.futex.CompareAndSwap(0, 1) {
			return
		}
	}
	for m.futex.Swap(2) != 0 {
		m.futex.Wait(2)
	}
}

func (m *hostMutex) Unlock() {
	if m.futex.Swap(0) == 2 {
		m.futex.Wake()
	}
}

type timerLock = hostMutex
type synctestLock = hostMutex
type channelLock = hostMutex

//export tinygo_worker_current
func currentWorker() *worker

//export tinygo_worker_init
func tinygo_worker_init(w *worker, mainStackSize *uintptr) int32

//export tinygo_worker_spawn
func tinygo_worker_spawn(w *worker, stackSize uintptr) int32

//export tinygo_worker_self
func tinygo_worker_self() uintptr

//export tinygo_worker_exit
func tinygo_worker_exit(w *worker)

//export tinygo_worker_calloc
func tinygo_worker_calloc(size uintptr) unsafe.Pointer

// Allocate a worker outside the GC heap. Allocating from the GC heap could
// start a GC cycle, which isn't possible with the scheduler lock held.
func newWorker() *worker {
	w := (*worker)(tinygo_worker_calloc(unsafe.Sizeof(worker{})))
	if w == nil {
		runtimeFatal("out of memory creating a worker thread")
	}
	return w
}

// Read GOMAXPROCS from the environment, or return 0 if it isn't set to a
// positive number. It doesn't allocate, so callers may hold the scheduler lock.
func envGOMAXPROCS() int32 {
	value := libc_getenv(unsafe.StringData("GOMAXPROCS\x00"))
	if value == nil {
		return 0
	}
	s := unsafe.Slice(value, strlen(unsafe.Pointer(value)))
	n := int32(0)
	for _, c := range s {
		if c < '0' || c > '9' || n > 100000 {
			return 0
		}
		n = n*10 + int32(c-'0')
	}
	return n
}

func run() {
	w := newWorker()
	numCPU = tinygo_worker_init(w, &w.stackBottom)
	w.thread = tinygo_worker_self()
	w.stackTop = stackTop
	w.stackBottom = stackTop - w.stackBottom
	allWorkers = w
	mainWorker = w

	initRand()
	initHeap()

	// Package initializers run on the main thread, like in Go. gomaxprocs is
	// still zero here, so no other worker starts and takes the main goroutine.
	go func() {
		initAll()
		unlockOSThread()
		callMain()
		// Other goroutines may still be running, so exit right away.
		exit(0)
	}()

	schedulerLock.Lock()
	gomaxprocs = numCPU
	if n := envGOMAXPROCS(); n > 0 {
		gomaxprocs = n
	}
	maxSchedulerThreads = int(gomaxprocs)
	t := runqueue.Pop()
	lockWorker(w, t)
	w.lockedInt = 1
	w.lockedReady = true
	schedule(false)
	schedulerLock.Unlock()
}

// Entry point of new worker threads.
//
//export tinygo_worker_start
func workerStart(w *worker) {
	w.thread = tinygo_worker_self()
	w.stackTop = getCurrentStackPointer()
	w.stackBottom = w.stackTop - workerStackSize

	// Make the GC aware of this worker before it runs any goroutine.
	workersLock.Lock()
	w.next = allWorkers
	allWorkers = w
	workersLock.Unlock()

	schedulerLock.Lock()
	schedule(false)
	schedulerLock.Unlock()
}

// Give a slot to the given worker. Call with the scheduler lock held.
func takeSlot(w *worker) {
	w.hasSlot = true
	slotsUsed++
}

func releaseSlot(w *worker) {
	w.hasSlot = false
	slotsUsed--
}

// Wait until another worker hands this worker a slot. Call with the scheduler
// lock held, it is held again on return.
func parkWorker(w *worker) {
	w.wake.Store(0)
	w.idleNext = idleWorkers
	idleWorkers = w
	schedulerLock.Unlock()
	for w.wake.Load() == 0 {
		w.wake.Wait(0)
	}
	schedulerLock.Lock()
}

// Make sure this worker holds a slot before it looks for work. Called at the
// start of each iteration of the scheduler loop with the lock held.
func schedulerAcquireSlot() {
	w := currentWorker()
	if w.locked != nil {
		runLocked(w)
	}
	if w.hasSlot {
		return
	}
	if slotsUsed < gomaxprocs {
		takeSlot(w)
		return
	}
	parkWorker(w)
}

// Wake a worker to run goroutines from the run queue, if there is a free slot.
// Call with the scheduler lock held.
func schedulerWake() {
	if slotsUsed >= gomaxprocs {
		// All slots are in use. Those workers will find the work.
		return
	}
	if w := idleWorkers; w != nil {
		idleWorkers = w.idleNext
		w.idleNext = nil
		takeSlot(w)
		w.wake.Store(1)
		w.wake.Wake()
		return
	}

	// No idle worker, so start a new one.
	w := newWorker()
	takeSlot(w)
	w.wake.Store(1)
	if tinygo_worker_spawn(w, workerStackSize) != 0 {
		runtimeFatal("failed to create a worker thread")
	}
}

// Mark the given task as runnable. Call with the scheduler lock held.
func schedulerQueue(t *task.Task) {
	if t.Affinity.Worker != 0 {
		(*worker)(unsafe.Pointer(t.Affinity.Worker)).lockedReady = true
		return
	}
	runqueue.Push(t)
}

// Wake a worker that may run the given task. Call with the scheduler lock
// held.
func schedulerWakeTask(t *task.Task) {
	if t.Affinity.Worker != 0 {
		w := (*worker)(unsafe.Pointer(t.Affinity.Worker))
		if w.lockedParked {
			w.lockedParked = false
			lockedRunning++
			w.wake.Store(1)
			w.wake.Wake()
		}
		return
	}
	schedulerWake()
}

func schedulerPop() *task.Task {
	return runqueue.Pop()
}

// Locked tasks only run on their own worker, which doesn't run this loop.
func schedulerCanRun(t *task.Task) bool {
	return t.Affinity.Worker == 0
}

func hasSleepingCore() bool {
	return clockWorker != nil
}

// Watch the clock for the given time, without a slot. Call with the scheduler
// lock held, it is held again on return.
func sleepTicksMulticore(d timeUnit) {
	w := currentWorker()
	releaseSlot(w)
	clockWorker = w
	const maxTime = timeUnit(1<<63 - 1)
	clockDeadline = ticks()
	if d > 0 && clockDeadline > maxTime-d {
		clockDeadline = maxTime
	} else {
		clockDeadline += d
	}
	w.wake.Store(0)
	schedulerLock.Unlock()
	w.wake.WaitUntil(0, uint64(ticksToNanoseconds(d)))
	schedulerLock.Lock()
	if clockWorker == w {
		clockWorker = nil
	}
}

// Make sure a worker watches the clock for the given wakeup time. Call with
// the scheduler lock held.
func interruptSleepTicksMulticore(wakeup timeUnit) {
	if w := clockWorker; w != nil {
		if wakeup < clockDeadline {
			clockDeadline = wakeup
			w.wake.Store(1)
			w.wake.Wake()
		}
		return
	}
	schedulerWake()
}

// Nothing to run. Park this worker until there is work. Call with the
// scheduler lock held, it is held again on return.
func schedulerUnlockAndWait() {
	w := currentWorker()
	releaseSlot(w)
	checkDeadlock()
	parkWorker(w)
}

// Abort if no worker runs and nothing can wake one. Call with the scheduler
// lock held.
func checkDeadlock() {
	if slotsUsed == 0 && inSyscall == 0 && lockedRunning == 0 && clockWorker == nil && runqueue.Empty() && sleepQueue == nil && timerQueuePeek() == nil {
		runtimeFatal("all goroutines are asleep - deadlock!")
	}
}

// Wake a worker if there is work that no worker takes care of. Call with the
// scheduler lock held.
func wakeForPendingWork() {
	if !runqueue.Empty() || ((sleepQueue != nil || timerQueuePeek() != nil) && clockWorker == nil) {
		schedulerWake()
	}
}

// Release the slot of this worker before a call that may block the thread, so
// that other goroutines can run in the meantime.
func entersyscall() {
	w := currentWorker()
	if w.task == nil || w.locked != nil {
		// Running in the scheduler (a timer callback), or on a locked worker
		// which has no slot.
		return
	}
	schedulerLock.Lock()
	releaseSlot(w)
	inSyscall++
	wakeForPendingWork()
	schedulerLock.Unlock()
}

// Take a slot again after a blocking call. If none is free, queue the current
// goroutine and park the worker.
func exitsyscall() {
	w := currentWorker()
	if w.task == nil || w.locked != nil {
		return
	}
	schedulerLock.Lock()
	inSyscall--
	if slotsUsed < gomaxprocs {
		takeSlot(w)
		schedulerLock.Unlock()
		return
	}
	// The scheduler loop parks this worker because it has no slot. A worker
	// with a slot picks up this goroutine.
	runqueue.Push(w.task)
	task.PauseLocked()
}

// Lock the given task to the worker. Call with the scheduler lock held.
func lockWorker(w *worker, t *task.Task) {
	w.locked = t
	t.Affinity.Worker = uintptr(unsafe.Pointer(w))
	lockedRunning++
	if w.hasSlot {
		releaseSlot(w)
	}
	wakeForPendingWork()
}

// Run the locked goroutine of this worker until it is unlocked. Call with the
// scheduler lock held, it is held again on return.
func runLocked(w *worker) {
	for w.locked != nil {
		if w.lockedExited {
			exitLockedWorker(w)
		}
		if !w.lockedReady {
			w.lockedParked = true
			lockedRunning--
			checkDeadlock()
			w.wake.Store(0)
			schedulerLock.Unlock()
			for w.wake.Load() == 0 {
				w.wake.Wait(0)
			}
			schedulerLock.Lock()
			continue
		}
		t := w.locked
		w.lockedReady = false
		setCurrentTask(t)
		t.RunState = task.RunStateRunning
		schedulerLock.Unlock()
		t.Resume()
		setCurrentTask(nil)
	}
}

// End the thread of a worker whose locked goroutine exited, like Go does.
// Call with the scheduler lock held.
func exitLockedWorker(w *worker) {
	w.locked = nil
	lockedRunning--
	checkDeadlock()
	schedulerLock.Unlock()
	if w == mainWorker {
		// The main thread can't exit without ending the process.
		for {
			w.wake.Store(0)
			w.wake.Wait(0)
		}
	}

	workersLock.Lock()
	for p := &allWorkers; *p != nil; p = &(*p).next {
		if *p == w {
			*p = w.next
			break
		}
	}
	workersLock.Unlock()
	tinygo_worker_exit(w)
}

// Called by the task package when a goroutine exits.
func workerTaskExit(t *task.Task) {
	if t.Affinity.Worker != 0 {
		currentWorker().lockedExited = true
	}
}

// LockOSThread wires the calling goroutine to its current operating system
// thread. Until it calls UnlockOSThread as often as LockOSThread, the goroutine
// runs only on this thread and no other goroutine runs on it. If the goroutine
// exits while locked, the thread exits too.
func LockOSThread() {
	w := lockOSThread()
	w.lockedExt++
	if w.lockedExt == 0 {
		runtimeFatal("LockOSThread nesting overflow")
	}
}

// UnlockOSThread undoes an earlier call to LockOSThread.
func UnlockOSThread() {
	w := currentWorker()
	if w.locked == nil || w.lockedExt == 0 {
		return
	}
	w.lockedExt--
	if w.lockedExt == 0 && w.lockedInt == 0 {
		unlockWorker(w)
	}
}

func lockOSThread() *worker {
	w := currentWorker()
	if w.locked == nil {
		schedulerLock.Lock()
		lockWorker(w, w.task)
		schedulerLock.Unlock()
	}
	return w
}

// Undo the lock by the runtime of the main goroutine during init.
func unlockOSThread() {
	w := currentWorker()
	w.lockedInt--
	if w.lockedExt == 0 && w.lockedInt == 0 {
		unlockWorker(w)
	}
}

// Unlock the current goroutine from its worker. It continues on this worker
// if there is a free slot, otherwise another worker picks it up.
func unlockWorker(w *worker) {
	t := w.task
	schedulerLock.Lock()
	w.locked = nil
	t.Affinity.Worker = 0
	lockedRunning--
	if slotsUsed < gomaxprocs {
		takeSlot(w)
		schedulerLock.Unlock()
		return
	}
	runqueue.Push(t)
	task.PauseLocked()
}

func NumCPU() int {
	return int(numCPU)
}

// GOMAXPROCS returns the number of slots. It is read from the GOMAXPROCS
// environment variable at startup, changing it later is ignored.
func GOMAXPROCS(n int) int {
	return int(gomaxprocs)
}

func currentTask() *task.Task {
	return currentWorker().task
}

func setCurrentTask(t *task.Task) {
	currentWorker().task = t
}

func systemStackPtr() *uintptr {
	return &currentWorker().systemStack
}

func lockFutex() interrupt.State {
	futexLock.Lock()
	return 0
}

func unlockFutex(state interrupt.State) {
	futexLock.Unlock()
}

func lockAtomics() interrupt.State {
	atomicsLock.Lock()
	return 0
}

func unlockAtomics(mask interrupt.State) {
	atomicsLock.Unlock()
}

func printlock() {
	printLock.Lock()
}

func printunlock() {
	printLock.Unlock()
}
