//go:build py32 && serial.uart

package machine

// Runtime output may run while the scheduler or allocator is locked.
func py32Putchar(c byte) bool {
	uart := Serial
	if uart == nil || uart.Bus == nil {
		return false
	}
	bus := uart.Bus
	retries := uart.txRetries
	for retries > 0 && !uartTXReady(bus) {
		retries--
	}
	if retries == 0 {
		return false
	}
	uartWriteData(bus, uint32(c))
	retries = uart.txRetries
	for retries > 0 && !uartTXComplete(bus) {
		retries--
	}
	return retries != 0
}
