// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reflect_test

import (
	"reflect"
	"strings"
	"testing"
)

func TestLongStructTags(t *testing.T) {
	typ := reflect.TypeOf(struct {
		Empty int
		Short int `x`
		At127 int `1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567`
		At128 int `12345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678`
		At255 int `123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345`
		At256 int `1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456`
	}{})
	for i, length := range []int{0, 1, 127, 128, 255, 256} {
		want := (strings.Repeat("1234567890", 26))[:length]
		if length == 1 {
			want = "x"
		}
		field := typ.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			if got := string(field.Tag); got != want {
				t.Errorf("Field(%d).Tag has length %d, want %d", i, len(got), length)
			}
			byName, ok := typ.FieldByName(field.Name)
			if !ok || string(byName.Tag) != want {
				t.Error("FieldByName returned an incorrect tag")
			}
			byFunc, ok := typ.FieldByNameFunc(func(name string) bool { return name == field.Name })
			if !ok || string(byFunc.Tag) != want {
				t.Error("FieldByNameFunc returned an incorrect tag")
			}
			if length != 0 && !strings.Contains(typ.String(), string(field.Tag)) {
				t.Error("Type.String omitted the tag")
			}
		})
	}
}

func TestFunctionResultCount(t *testing.T) {
	type manyResults func() (
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
	)
	type manyVariadicResults func(...int) (
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
		int, int, int, int, int, int, int, int,
	)
	for i, typ := range []reflect.Type{reflect.TypeFor[manyResults](), reflect.TypeFor[manyVariadicResults]()} {
		if typ.NumIn() != i || typ.NumOut() != 128 || typ.IsVariadic() != (i != 0) {
			t.Fatalf("function %v has %d inputs, %d outputs, variadic %v", typ, typ.NumIn(), typ.NumOut(), typ.IsVariadic())
		}
		for n := 0; n < typ.NumOut(); n++ {
			if got := typ.Out(n); got != reflect.TypeFor[int]() {
				t.Fatalf("Out(%d) = %v, want int", n, got)
			}
		}
	}
}

func checkConstructedType[T any](t *testing.T, value reflect.Value) {
	t.Helper()
	if _, ok := value.Interface().(T); !ok {
		t.Fatalf("constructed %v failed a concrete type assertion", value.Type())
	}
	switch value.Interface().(type) {
	case T:
	default:
		t.Fatalf("constructed %v failed a type switch", value.Type())
	}
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
