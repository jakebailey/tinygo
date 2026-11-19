package main

import "time"

func main() {
	checkTimer(func() { time.NewTimer(time.Second) })
	checkTimer(func() { time.AfterFunc(time.Second, func() { println("callback ran") }) })
}

func checkTimer(f func()) {
	defer func() {
		err, ok := recover().(error)
		if !ok {
			panic("expected a timer error")
		}
		println(err.Error())
	}()
	f()
}
