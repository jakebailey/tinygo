package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	fd := os.NewFile(3, "socket").PollFD()
	if err := fd.Init("tcp", true); err != nil {
		panic(err)
	}
	if err := fd.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		panic(err)
	}
	start := time.Now()
	n, _, op, err := fd.Accept()
	if time.Since(start) < 10*time.Millisecond {
		panic("deadline woke early")
	}
	fmt.Printf("accept: %d %s %v\n", n, op, err)
}
