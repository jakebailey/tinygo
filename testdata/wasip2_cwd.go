package main

import (
	"os"
	"path"
	"runtime"
)

var initialCWD string

func init() {
	var err error
	initialCWD, err = os.Getwd()
	if err != nil || initialCWD != os.Getenv("EXPECT_CWD") {
		panic("unexpected initial cwd: " + initialCWD)
	}
	if data, err := os.ReadFile("filesystem.txt"); err != nil || len(data) != 18 {
		panic("relative read during init failed")
	}
}

func main() {
	runtime.GC()
	if err := os.Chdir(".."); err != nil {
		panic(err)
	}
	changed, err := os.Getwd()
	if err != nil || path.Clean(changed) != path.Dir(path.Clean(initialCWD)) {
		panic("unexpected cwd after Chdir: " + changed)
	}
	if initialCWD != os.Getenv("EXPECT_CWD") {
		panic("Getwd result changed after Chdir")
	}
}
