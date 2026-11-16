package main

import "time"

func main() {
	time.Sleep(time.Nanosecond)
	done := make(chan struct{})
	go func() {
		time.Sleep(time.Duration(1<<63 - 1))
		close(done)
	}()
	select {
	case <-done:
		println("oversized sleep returned early")
	case <-time.After(time.Millisecond):
		println("oversized sleep stayed blocked")
	}
}
