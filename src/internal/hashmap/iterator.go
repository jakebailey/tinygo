package hashmap

import "unsafe"

type Iterator struct {
	Buckets      unsafe.Pointer
	NumBuckets   uintptr
	BucketNumber uintptr
	StartBucket  uintptr
	Bucket       unsafe.Pointer
	BucketIndex  uint8
	StartIndex   uint8
	Wrapped      bool
	Key          unsafe.Pointer
	Value        unsafe.Pointer
}
