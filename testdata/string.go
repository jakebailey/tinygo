package main

func testRangeString() {
	for i, c := range "abcü¢€𐍈°x" {
		println(i, c)
	}
}

func testStringToRunes() {
	var s = "abcü¢€𐍈°x"
	for i, c := range []rune(s) {
		println(i, c)
	}
}

func testRunesToString(r []rune) {
	println("string from runes:", string(r))
}

func testByteToString() {
	for i := 0; i < 256; i++ {
		buf := []byte{byte(i)}
		str := string(buf)
		buf[0] = 0
		if len(str) != 1 || str[0] != byte(i) {
			panic("incorrect one-byte string")
		}
	}
	if string([]byte{}) != "" {
		panic("incorrect empty string")
	}
}

type myString string

func main() {
	testRangeString()
	testStringToRunes()
	testRunesToString([]rune{97, 98, 99, 252, 162, 8364, 66376, 176, 120})
	testByteToString()
	var _ = len([]byte(myString("foobar"))) // issue 1246
}
