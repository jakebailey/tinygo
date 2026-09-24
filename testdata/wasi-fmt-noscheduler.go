package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("hello")
	fmt.Printf("%s\n", "world")
	fmt.Fprintln(os.Stdout, "from stdout")
	var n int
	if _, err := fmt.Sscanf("17", "%d", &n); err != nil {
		panic(err)
	}
	fmt.Println(n)
}
