// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reflect_test

import (
	"reflect"
	"runtime"
	"testing"
)

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
