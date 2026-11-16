//go:build scheduler.workers

package runtime

// The workers scheduler runs goroutines on a pool of OS threads (workers).
// At most gomaxprocs workers run goroutines at the same time: each of them
// holds a slot. A worker without a slot is idle, watches the clock, or is
// blocked in a system call.
//
// Goroutines switch stacks cooperatively like with the cores scheduler, and
// share the run queue, sleep queue and timers with it (scheduler_multicore.go).
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

	// The pthread_t of this worker.
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

	// Stack pointer where the worker stopped for the GC.
	gcSP uintptr
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

	gomaxprocs = numCPU
	if n := envGOMAXPROCS(); n > 0 {
		gomaxprocs = n
	}
	maxSchedulerThreads = int(gomaxprocs)

	// The main thread starts out running goroutines.
	w.hasSlot = true
	slotsUsed = 1

	initRand()
	initHeap()

	go func() {
		initAll()
		callMain()
		// Other goroutines may still be running, so exit right away.
		exit(0)
	}()

	schedulerLock.Lock()
	scheduler(false)
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
	scheduler(false)
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

func schedulerWakeTask(t *task.Task) {
	schedulerWake()
}

func schedulerPop() *task.Task {
	return runqueue.Pop()
}

func schedulerCanRun(t *task.Task) bool {
	return true
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
	if slotsUsed == 0 && inSyscall == 0 && clockWorker == nil && runqueue.Empty() && sleepQueue == nil && timerQueuePeek() == nil {
		// No worker is running and nothing can wake one.
		runtimeFatal("all goroutines are asleep - deadlock!")
	}
	parkWorker(w)
}

// Release the slot of this worker before a call that may block the thread, so
// that other goroutines can run in the meantime.
func entersyscall() {
	w := currentWorker()
	if w.task == nil {
		// Running in the scheduler (a timer callback), don't touch the slot.
		return
	}
	schedulerLock.Lock()
	releaseSlot(w)
	inSyscall++
	if !runqueue.Empty() || ((sleepQueue != nil || timerQueuePeek() != nil) && clockWorker == nil) {
		schedulerWake()
	}
	schedulerLock.Unlock()
}

// Take a slot again after a blocking call. If none is free, queue the current
// goroutine and park the worker.
func exitsyscall() {
	w := currentWorker()
	if w.task == nil {
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
