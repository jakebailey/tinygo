// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reflect_test

import (
	"reflect"
	"runtime"
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

func TestTypeFieldByIndexEmpty(t *testing.T) {
	type item struct {
		A string `json:"a"`
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[item](),
		reflect.TypeFor[struct{}](),
	} {
		t.Run(typ.String(), func(t *testing.T) {
			for _, test := range []struct {
				name  string
				index []int
			}{
				{"nil", nil},
				{"empty", []int{}},
			} {
				t.Run(test.name, func(t *testing.T) {
					field := typ.FieldByIndex(test.index)
					if field.Type != typ {
						t.Fatal("FieldByIndex returned a different type")
					}
					if got := field.Type.Kind(); got != reflect.Struct {
						t.Fatalf("unexpected field kind: got %v want %v", got, reflect.Struct)
					}
					if want := (reflect.StructField{Type: typ}); !reflect.DeepEqual(field, want) {
						t.Fatalf("unexpected field result\ngot %#v\nwant %#v", field, want)
					}
				})
			}
		})
	}
}

func TestTypeFieldByIndexFields(t *testing.T) {
	type fields struct {
		Pad    byte
		Name   string `json:"name"`
		hidden int
	}
	typ := reflect.TypeFor[fields]()
	embedded := reflect.TypeFor[struct {
		Pad byte
		fields
	}]()
	pointer := reflect.TypeFor[struct {
		Pad byte
		*fields
	}]()
	for _, test := range []struct {
		name  string
		typ   reflect.Type
		index []int
		want  reflect.StructField
	}{
		{"direct", typ, []int{1}, typ.Field(1)},
		{"unexported", typ, []int{2}, typ.Field(2)},
		{"embedded", embedded, []int{1}, embedded.Field(1)},
		{"promoted", embedded, []int{1, 1}, typ.Field(1)},
		{"pointer", pointer, []int{1}, pointer.Field(1)},
		{"pointer-promoted", pointer, []int{1, 1}, typ.Field(1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			field := test.typ.FieldByIndex(test.index)
			if field.Type != test.want.Type {
				t.Fatal("FieldByIndex returned a different type")
			}
			if got, want := field.Type.Kind(), test.want.Type.Kind(); got != want {
				t.Fatalf("unexpected field kind: got %v want %v", got, want)
			}
			field.Index = test.want.Index
			if !reflect.DeepEqual(field, test.want) {
				t.Fatalf("unexpected field result\ngot %#v\nwant %#v", field, test.want)
			}
		})
	}
}

func TestTypeFieldByIndexNonStruct(t *testing.T) {
	if runtime.GOARCH == "wasm" {
		t.Log("recover is not supported on wasm")
		return
	}

	type item struct {
		A string
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[int](),
		reflect.TypeFor[*item](),
	} {
		t.Run(typ.String(), func(t *testing.T) {
			for _, test := range []struct {
				name  string
				index []int
			}{
				{"nil", nil},
				{"empty", []int{}},
				{"field", []int{0}},
			} {
				t.Run(test.name, func(t *testing.T) {
					defer func() { recover() }()
					typ.FieldByIndex(test.index)
					t.Fatal("FieldByIndex did not panic")
				})
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

func TestCanSeqFunction(t *testing.T) {
	type namedBool bool

	tests := []struct {
		name     string
		typ      reflect.Type
		wantSeq  bool
		wantSeq2 bool
	}{
		{"seq", reflect.TypeOf(func(func(int) bool) {}), true, false},
		{"seq2", reflect.TypeOf(func(func(int, string) bool) {}), false, true},
		{"no result", reflect.TypeOf(func(func(int)) {}), false, false},
		{"named bool", reflect.TypeOf(func(func(int) namedBool) {}), false, false},
		{"outer result", reflect.TypeOf(func(func(int) bool) bool { return false }), false, false},
		{"two inputs", reflect.TypeOf(func(func(int) bool, int) {}), false, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.typ.CanSeq(); got != test.wantSeq {
				t.Errorf("CanSeq() = %v, want %v", got, test.wantSeq)
			}
			if got := test.typ.CanSeq2(); got != test.wantSeq2 {
				t.Errorf("CanSeq2() = %v, want %v", got, test.wantSeq2)
			}
		})
	}
}
