//go:build scheduler.cores

package main

import "unsafe"

//go:linkname currentTask runtime.currentTask
func currentTask() unsafe.Pointer
