package main

var wasmGCMessage = "wasmgc!"

func middleWasmGCString(value string) string {
	return value[2:7]
}

func sumWasmGCString(value string) int {
	total := 0
	for i := 0; i < len(value); i++ {
		total += int(value[i])
	}
	return total
}

func incrementWasmGCByte(value byte) byte {
	return value + 1
}

func main() {
	collectionPoint := make([]int, 1)
	collectionPoint[0] = 1

	value := middleWasmGCString(wasmGCMessage)
	println(len(value), value[0], sumWasmGCString(value), value == "smgc!")

	number := 300
	println(len(""[0:0]), "abc"[1], incrementWasmGCByte(255), byte(number))
}
