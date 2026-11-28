//go:build !stack.asyncify

package runtime

func stackReached(header uintptr) {}
