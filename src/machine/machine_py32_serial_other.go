//go:build py32 && !serial.uart

package machine

func py32Putchar(c byte) bool {
	return Serial.WriteByte(c) == nil
}
