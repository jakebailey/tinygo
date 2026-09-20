// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reflect_test

import (
	"reflect"
	"runtime"
	"testing"
)

func TestArrayOfRuntimeConstruction(t *testing.T) {
	type elem struct {
		Value *int
	}

	elemType := reflect.TypeOf(elem{})
	compiledType := reflect.TypeOf([3]elem{})
	if got := reflect.ArrayOf(3, elemType); got != compiledType {
		t.Fatalf("ArrayOf(3, elem) = %v, want existing type %v", got, compiledType)
	}

	arrayType := reflect.ArrayOf(4, elemType)
	if got, want := arrayType.String(), "[4]reflect_test.elem"; got != want {
		t.Fatalf("ArrayOf(4, elem).String() = %q, want %q", got, want)
	}
	if got := arrayType.Kind(); got != reflect.Array {
		t.Fatalf("ArrayOf(4, elem).Kind() = %v, want %v", got, reflect.Array)
	}
	if got := arrayType.Len(); got != 4 {
		t.Fatalf("ArrayOf(4, elem).Len() = %d, want 4", got)
	}
	if got := arrayType.Elem(); got != elemType {
		t.Fatalf("ArrayOf(4, elem).Elem() = %v, want %v", got, elemType)
	}
	if got := reflect.ArrayOf(4, elemType); got != arrayType {
		t.Fatalf("second ArrayOf(4, elem) = %v, want %v", got, arrayType)
	}
	if got := reflect.PointerTo(arrayType).Elem(); got != arrayType {
		t.Fatalf("PointerTo(ArrayOf(4, elem)).Elem() = %v, want %v", got, arrayType)
	}

	value := 42
	array := reflect.New(arrayType).Elem()
	array.Index(0).Field(0).Set(reflect.ValueOf(&value))
	runtime.GC()
	if got := array.Index(0).Field(0).Elem().Int(); got != int64(value) {
		t.Fatalf("array element after GC = %d, want %d", got, value)
	}

	checkPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		fn()
	}
	checkPanic("negative ArrayOf length", func() {
		reflect.ArrayOf(-1, elemType)
	})
	checkPanic("overflowing ArrayOf size", func() {
		reflect.ArrayOf(int(^uint(0)>>1), reflect.TypeOf(int(0)))
	})
}

func TestChanOfRuntimeConstruction(t *testing.T) {
	type elem int

	elemType := reflect.TypeOf(elem(0))
	compiledTypes := []struct {
		dir  reflect.ChanDir
		want reflect.Type
	}{
		{reflect.RecvDir, reflect.TypeOf((<-chan elem)(nil))},
		{reflect.SendDir, reflect.TypeOf((chan<- elem)(nil))},
		{reflect.BothDir, reflect.TypeOf((chan elem)(nil))},
	}
	for _, test := range compiledTypes {
		if got := reflect.ChanOf(test.dir, elemType); got != test.want {
			t.Errorf("ChanOf(%v, elem) = %v, want existing type %v", test.dir, got, test.want)
		}
	}

	dynamicElemType := reflect.ArrayOf(3, reflect.TypeOf(uint8(0)))
	dynamicTypes := []struct {
		dir  reflect.ChanDir
		name string
	}{
		{reflect.RecvDir, "<-chan [3]uint8"},
		{reflect.SendDir, "chan<- [3]uint8"},
		{reflect.BothDir, "chan [3]uint8"},
	}
	var chanType reflect.Type
	for _, test := range dynamicTypes {
		typ := reflect.ChanOf(test.dir, dynamicElemType)
		if got := typ.String(); got != test.name {
			t.Errorf("ChanOf(%v, [3]uint8).String() = %q, want %q", test.dir, got, test.name)
		}
		if got := typ.ChanDir(); got != test.dir {
			t.Errorf("ChanOf(%v, [3]uint8).ChanDir() = %v", test.dir, got)
		}
		if got := typ.Elem(); got != dynamicElemType {
			t.Errorf("ChanOf(%v, [3]uint8).Elem() = %v, want %v", test.dir, got, dynamicElemType)
		}
		if got := reflect.ChanOf(test.dir, dynamicElemType); got != typ {
			t.Errorf("second ChanOf(%v, [3]uint8) = %v, want %v", test.dir, got, typ)
		}
		if test.dir == reflect.BothDir {
			chanType = typ
		}
	}
	if got := chanType.Kind(); got != reflect.Chan {
		t.Fatalf("ChanOf(BothDir, [3]uint8).Kind() = %v, want %v", got, reflect.Chan)
	}
	if !chanType.Comparable() {
		t.Fatal("ChanOf(BothDir, [3]uint8).Comparable() = false, want true")
	}
	if got := reflect.PointerTo(chanType).Elem(); got != chanType {
		t.Fatalf("PointerTo(ChanOf(BothDir, [3]uint8)).Elem() = %v, want %v", got, chanType)
	}
	channel := reflect.MakeChan(chanType, 3)
	if got := channel.Cap(); got != 3 {
		t.Fatalf("MakeChan(ChanOf(BothDir, [3]uint8), 3).Cap() = %d, want 3", got)
	}
	channelMap := reflect.MakeMap(reflect.MapOf(chanType, reflect.TypeOf(int(0))))
	channelMap.SetMapIndex(channel, reflect.ValueOf(42))
	if got := channelMap.MapIndex(channel).Int(); got != 42 {
		t.Fatalf("map value with dynamic channel key = %d, want 42", got)
	}

	checkPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		fn()
	}
	checkPanic("invalid ChanDir", func() {
		reflect.ChanOf(0, elemType)
	})
	checkPanic("oversized channel element", func() {
		reflect.ChanOf(reflect.BothDir, reflect.TypeOf((*[1 << 16]byte)(nil)).Elem())
	})
}

func TestMapOfRuntimeConstruction(t *testing.T) {
	type key string
	type elem float64

	keyType := reflect.TypeOf(key(""))
	elemType := reflect.TypeOf(elem(0))
	compiledType := reflect.TypeOf(map[key]elem(nil))
	mapType := reflect.MapOf(keyType, elemType)
	if mapType != compiledType {
		t.Fatalf("MapOf(key, elem) = %v, want existing type %v", mapType, compiledType)
	}
	if got, want := mapType.String(), "map[reflect_test.key]reflect_test.elem"; got != want {
		t.Fatalf("MapOf(key, elem).String() = %q, want %q", got, want)
	}
	if got := mapType.Key(); got != keyType {
		t.Fatalf("MapOf(key, elem).Key() = %v, want %v", got, keyType)
	}
	if got := mapType.Elem(); got != elemType {
		t.Fatalf("MapOf(key, elem).Elem() = %v, want %v", got, elemType)
	}
	if got := reflect.MapOf(keyType, elemType); got != mapType {
		t.Fatalf("second MapOf(key, elem) = %v, want %v", got, mapType)
	}

	m := reflect.MakeMap(mapType)
	m.SetMapIndex(reflect.ValueOf(key("a")), reflect.ValueOf(elem(1)))
	runtime.GC()
	if got := m.MapIndex(reflect.ValueOf(key("a"))).Float(); got != 1 {
		t.Fatalf("constructed map value = %v, want 1", got)
	}

	pointerType := reflect.TypeOf((*int)(nil))
	pointerMap := reflect.MakeMap(reflect.MapOf(pointerType, pointerType))
	keyPointer := new(int)
	valuePointer := new(int)
	*valuePointer = 42
	pointerMap.SetMapIndex(reflect.ValueOf(keyPointer), reflect.ValueOf(valuePointer))
	runtime.GC()
	if got := pointerMap.MapIndex(reflect.ValueOf(keyPointer)).Elem().Int(); got != 42 {
		t.Fatalf("pointer map element after GC = %d, want 42", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("MapOf accepted an invalid key type")
		}
	}()
	reflect.MapOf(reflect.TypeOf((func())(nil)), reflect.TypeOf(false))
}

func TestTypeFor(t *testing.T) {
	type (
		mystring string
		myiface  interface{}
	)

	testcases := []struct {
		wantFrom any
		got      reflect.Type
	}{
		{new(int), reflect.TypeFor[int]()},
		{new(int64), reflect.TypeFor[int64]()},
		{new(string), reflect.TypeFor[string]()},
		{new(mystring), reflect.TypeFor[mystring]()},
		{new(any), reflect.TypeFor[any]()},
		{new(myiface), reflect.TypeFor[myiface]()},
	}
	for _, tc := range testcases {
		want := reflect.ValueOf(tc.wantFrom).Elem().Type()
		if want != tc.got {
			t.Errorf("unexpected reflect.Type: got %v; want %v", tc.got, want)
		}
	}
}
func TestElemOfNamedMultiPointer(t *testing.T) {
	type recursive ***recursive

	tests := []struct {
		typ  reflect.Type
		want reflect.Type
	}{
		{reflect.TypeFor[recursive](), reflect.TypeFor[**recursive]()},
		{reflect.TypeFor[**recursive](), reflect.TypeFor[*recursive]()},
		{reflect.TypeFor[*recursive](), reflect.TypeFor[recursive]()},
	}
	for _, test := range tests {
		if got := test.typ.Elem(); got != test.want {
			t.Errorf("%v.Elem() = %v; want %v", test.typ, got, test.want)
		}
	}
}

func TestSliceOfRuntimeConstruction(t *testing.T) {
	type compiledElem int
	compiledType := reflect.TypeOf([]compiledElem(nil))
	if got := reflect.SliceOf(reflect.TypeOf(compiledElem(0))); got != compiledType {
		t.Fatalf("SliceOf(compiledElem) = %v, want existing type %v", got, compiledType)
	}

	type elem struct {
		Value *int
	}

	elemType := reflect.TypeOf(elem{})
	sliceType := reflect.SliceOf(elemType)
	if got, want := sliceType.String(), "[]reflect_test.elem"; got != want {
		t.Fatalf("SliceOf(elem).String() = %q, want %q", got, want)
	}
	if got := sliceType.Kind(); got != reflect.Slice {
		t.Fatalf("SliceOf(elem).Kind() = %v, want %v", got, reflect.Slice)
	}
	if got := sliceType.Elem(); got != elemType {
		t.Fatalf("SliceOf(elem).Elem() = %v, want %v", got, elemType)
	}
	if got := reflect.SliceOf(elemType); got != sliceType {
		t.Fatalf("second SliceOf(elem) = %v, want %v", got, sliceType)
	}
	if got := reflect.PointerTo(sliceType).Elem(); got != sliceType {
		t.Fatalf("PointerTo(SliceOf(elem)).Elem() = %v, want %v", got, sliceType)
	}

	value := 42
	slice := reflect.MakeSlice(sliceType, 1, 1)
	slice.Index(0).Field(0).Set(reflect.ValueOf(&value))
	runtime.GC()
	if got := slice.Index(0).Field(0).Elem().Int(); got != int64(value) {
		t.Fatalf("slice element after GC = %d, want %d", got, value)
	}
}
