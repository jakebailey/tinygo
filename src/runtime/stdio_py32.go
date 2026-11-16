//go:build py32

package runtime

import (
	"machine"
	_ "unsafe"
)

func putchar(c byte) {
	if !py32Putchar(c) {
		// A serial failure cannot be reported through serial.
		trap()
	}
}

//go:linkname py32Putchar machine.py32Putchar
func py32Putchar(c byte) bool

func getchar() byte {
	for machine.Serial.Buffered() == 0 {
		Gosched()
	}
	v, _ := machine.Serial.ReadByte()
	return v
}

func buffered() int {
	return machine.Serial.Buffered()
}
