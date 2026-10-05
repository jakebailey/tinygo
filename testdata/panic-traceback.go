package main

import "fmt"

var mode string

//go:noinline
func panicHere() {
	panic("boom")
}

var panicFunc = panicHere

//go:noinline
func inner() {
	defer func() {}()
	panicFunc()
}

//go:noinline
func outer() {
	defer func() {}()
	inner()
}

func inlinePanic() int {
	panic("boom")
}

func direct() {
	defer func() {}()
	_ = inlinePanic()
}

func panicPointer(value *int, err error) *int {
	if err != nil {
		panic("boom")
	}
	return value
}

func crossPackage() {
	defer func() {}()
	_ = panicPointer(nil, fmt.Errorf("boom"))
}

func main() {
	if mode == "direct" {
		direct()
		return
	}
	if mode == "cross-package" {
		crossPackage()
		return
	}
	defer func() {}()
	outer()
}
