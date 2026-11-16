//go:build scheduler.unified && (exec.single || exec.asyncify || exec.multicore)

package runtime

// run initializes the program and runs the scheduler.
func run() {
	initRand()
	initHeap()

	go func() {
		initAll()
		schedulerStartCores()
		callMain()
		schedulerSetMainExited()
	}()

	scheduler(false)
}
