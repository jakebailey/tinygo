//go:build scheduler.tasks

package main

import "unsafe"

//go:linkname currentTask internal/task.Current
func currentTask() unsafe.Pointer
