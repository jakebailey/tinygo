//go:build !((linux || darwin) && scheduler.threads && (386 || amd64 || arm64))

package runtime

func Callers(skip int, pc []uintptr) int {
	return 0
}
