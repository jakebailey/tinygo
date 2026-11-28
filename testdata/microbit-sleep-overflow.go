package main

import (
	"device/arm"
	"runtime"
	"time"
)

func main() {
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		time.Sleep(time.Duration(1<<63 - 1))
		close(done)
	}()
	<-started
	for i := 0; i < 10; i++ {
		runtime.Gosched()
		select {
		case <-done:
			println("oversized sleep returned early")
			arm.SemihostingCall(arm.SemihostingReportException, arm.SemihostingApplicationExit)
			return
		default:
		}
	}
	println("oversized sleep stayed blocked")
	arm.SemihostingCall(arm.SemihostingReportException, arm.SemihostingApplicationExit)
}
