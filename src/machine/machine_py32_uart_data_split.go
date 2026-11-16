//go:build py32 && py32_usart_split_data

package machine

import "device/py32"

func readUSARTData(bus *py32.USART_Type) uint32 {
	return bus.RDR.Get()
}

func uartWriteData(bus *py32.USART_Type, value uint32) {
	bus.TDR.Set(value)
}

func uartTXReady(bus *py32.USART_Type) bool {
	return bus.SR.Get()&py32.USART_SR_TXE_TXFNF != 0
}

func uartTXComplete(bus *py32.USART_Type) bool {
	return bus.SR.Get()&py32.USART_SR_TC != 0
}
