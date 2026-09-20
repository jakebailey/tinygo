// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reflect_test

import (
	"reflect"
	"testing"
)

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
