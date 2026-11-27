package main

type storageValue struct {
	data [4096]byte
	ptr  *byte
}

type storageWithBlank struct {
	data  [4096]byte
	_     [16]byte
	score float64
}

func resetStorage(value *storageValue) {
	*value = storageValue{}
}

func resetArray(value *[4096]byte) {
	*value = [4096]byte{}
}

func resetSmallArray(value *[4]byte) {
	*value = [4]byte{}
}

func copyStorage(dst, src *storageValue) {
	value := *src
	src.data[0] = 42
	*dst = value
}

func compareArray(src, other *[4096]byte) bool {
	value := *src
	same := value == *other
	src[0] = 42
	return same
}

func passStorage(value storageValue) byte {
	return value.data[0]
}

func compareStorage(x, y *storageValue) bool {
	return *x == *y
}

func compareZeroStorage(x *storageValue) bool {
	return *x == storageValue{}
}

func compareStorageWithBlank(x, y *storageWithBlank) bool {
	return *x == *y
}

func compareNonBinaryArray(x, y *[1025]float64) bool {
	return *x == *y
}

func compareStoragePhi(x, y *storageValue, flag bool) bool {
	return *x == *y && flag
}

func compareNonBinaryArrayPhi(x, y *[1025]float64, flag bool) bool {
	return *x == *y && flag
}
