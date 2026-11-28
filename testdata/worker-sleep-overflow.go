package main

import "time"

func main() {
	started := make(chan struct{})
	go func() {
		close(started)
		time.Sleep(time.Duration(1<<63 - 1))
		println("oversized sleep returned early")
	}()
	<-started
	time.Sleep(10 * time.Millisecond)
	// Give another worker time to watch the oversized sleep.
	until := time.Now().Add(20 * time.Millisecond)
	for time.Now().Before(until) {
	}
	<-time.After(time.Millisecond)
	println("earlier timer fired")
}
