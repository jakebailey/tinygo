//go:build linux

package builder

/*
#include <stdlib.h>

#if defined(__GLIBC__)
#include <malloc.h>
static void tinygo_malloc_trim(void) {
	malloc_trim(0);
}
#else
static void tinygo_malloc_trim(void) {
}
#endif
*/
import "C"

func trimHeap() {
	C.tinygo_malloc_trim()
}
