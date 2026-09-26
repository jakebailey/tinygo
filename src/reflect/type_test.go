// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reflect_test

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

type StructOfEmbedded struct {
	Value int
}

type StructOfEmbeddedWithMethod struct{}

func (StructOfEmbeddedWithMethod) Method() int { return 42 }

type structOfMethod interface {
	Method() int
}

type StructOfEmbeddedInterface interface {
	Method() int
}

type StructOfEmbeddedPointer struct {
	Value int
}

func (v *StructOfEmbeddedPointer) Method() int { return v.Value }
func (v *StructOfEmbeddedPointer) Set(value int) {
	v.Value = value
}

type structOfSetter interface {
	Set(int)
}

type structOfPair [2]uintptr

type StructOfEmbeddedABI struct {
	Base int
}

func (v StructOfEmbeddedABI) Mix(a byte, b int, pair structOfPair, text string) (byte, int, structOfPair, string) {
	return a, v.Base + b, pair, text
}

func (v StructOfEmbeddedABI) Sum(values ...int) int {
	total := v.Base
	for _, value := range values {
		total += value
	}
	return total
}

type structOfABI interface {
	Mix(byte, int, structOfPair, string) (byte, int, structOfPair, string)
	Sum(...int) int
}

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

func TestStructOfRuntimeConstruction(t *testing.T) {
	uint64Type := reflect.TypeOf(uint64(0))
	if got, want := reflect.StructOf([]reflect.StructField{{Name: "Y", Type: uint64Type}}), reflect.TypeOf(struct{ Y uint64 }{}); got != want {
		t.Fatalf("StructOf({Y uint64}) = %v, want existing type %v", got, want)
	}
	if got, want := reflect.StructOf(nil), reflect.TypeOf(struct{}{}); got != want {
		t.Fatalf("StructOf(nil) = %v, want existing type %v", got, want)
	}

	pointerType := reflect.TypeOf((*int)(nil))
	arrayType := reflect.ArrayOf(2, pointerType)
	fields := []reflect.StructField{
		{Name: "Byte", Type: reflect.TypeOf(byte(0))},
		{Name: "Pointer", Type: pointerType},
		{Name: "Pointers", Type: arrayType, Tag: `json:"pointers"`},
	}
	structType := reflect.StructOf(fields)
	if got, want := structType.String(), `struct { Byte uint8; Pointer *int; Pointers [2]*int "json:\"pointers\"" }`; got != want {
		t.Fatalf("StructOf(fields).String() = %q, want %q", got, want)
	}
	if got := reflect.StructOf(fields); got != structType {
		t.Fatalf("second StructOf(fields) = %v, want %v", got, structType)
	}
	if got := reflect.PointerTo(structType).Elem(); got != structType {
		t.Fatalf("PointerTo(StructOf(fields)).Elem() = %v, want %v", got, structType)
	}
	if !structType.Comparable() {
		t.Fatal("StructOf(fields).Comparable() = false, want true")
	}

	type layout struct {
		A byte
		B *int
		C [2]*int
	}
	if got, want := structType.Size(), unsafe.Sizeof(layout{}); got != want {
		t.Fatalf("StructOf(fields).Size() = %d, want %d", got, want)
	}
	if got, want := structType.Align(), int(unsafe.Alignof(layout{})); got != want {
		t.Fatalf("StructOf(fields).Align() = %d, want %d", got, want)
	}
	wantOffsets := []uintptr{
		unsafe.Offsetof(layout{}.A),
		unsafe.Offsetof(layout{}.B),
		unsafe.Offsetof(layout{}.C),
	}
	for i, want := range wantOffsets {
		if got := structType.Field(i).Offset; got != want {
			t.Errorf("StructOf(fields).Field(%d).Offset = %d, want %d", i, got, want)
		}
	}
	if got := structType.Field(2).Tag.Get("json"); got != "pointers" {
		t.Fatalf("StructOf(fields).Field(2).Tag.Get(\"json\") = %q, want %q", got, "pointers")
	}

	type trailingZeroLayout struct {
		A byte
		B [0]uint64
	}
	trailingZeroType := reflect.StructOf([]reflect.StructField{
		{Name: "Byte", Type: reflect.TypeOf(byte(0))},
		{Name: "Zero", Type: reflect.TypeOf([0]uint64{})},
	})
	if got, want := trailingZeroType.Size(), unsafe.Sizeof(trailingZeroLayout{}); got != want {
		t.Fatalf("StructOf with trailing zero-sized field has size %d, want %d", got, want)
	}

	first := 41
	second := 42
	value := reflect.New(structType).Elem()
	value.Field(1).Set(reflect.ValueOf(&first))
	value.Field(2).Index(1).Set(reflect.ValueOf(&second))
	runtime.GC()
	if got := value.Field(1).Elem().Int(); got != int64(first) {
		t.Fatalf("first pointer field after GC = %d, want %d", got, first)
	}
	if got := value.Field(2).Index(1).Elem().Int(); got != int64(second) {
		t.Fatalf("array pointer field after GC = %d, want %d", got, second)
	}
	iface := value.Interface()
	runtime.GC()
	if got := reflect.ValueOf(iface).Field(1).Elem().Int(); got != int64(first) {
		t.Fatalf("interface struct pointer field after GC = %d, want %d", got, first)
	}
	if iface != value.Interface() {
		t.Fatal("dynamic struct value is not equal to itself")
	}

	structMap := reflect.MakeMap(reflect.MapOf(structType, reflect.TypeOf(int(0))))
	structMap.SetMapIndex(value, reflect.ValueOf(7))
	if got := structMap.MapIndex(value).Int(); got != 7 {
		t.Fatalf("map value with dynamic struct key = %d, want 7", got)
	}

	embeddedType := reflect.StructOf([]reflect.StructField{{
		Name:      "StructOfEmbedded",
		Type:      reflect.TypeOf(StructOfEmbedded{}),
		Anonymous: true,
	}})
	if got, want := embeddedType.String(), "struct { reflect_test.StructOfEmbedded }"; got != want {
		t.Fatalf("embedded StructOf.String() = %q, want %q", got, want)
	}
	embedded := reflect.New(embeddedType).Elem()
	embedded.FieldByName("Value").SetInt(9)
	if got := embedded.Field(0).Field(0).Int(); got != 9 {
		t.Fatalf("promoted embedded field = %d, want 9", got)
	}

	unexportedType := reflect.StructOf([]reflect.StructField{{
		Name:    "value",
		PkgPath: "reflect_test",
		Type:    reflect.TypeOf(int(0)),
	}})
	if got := unexportedType.Field(0).PkgPath; got != "reflect_test" {
		t.Fatalf("unexported field PkgPath = %q, want %q", got, "reflect_test")
	}

	if reflect.StructOf([]reflect.StructField{{
		Name: "Values",
		Type: reflect.TypeOf([]int(nil)),
	}}).Comparable() {
		t.Fatal("StructOf with slice field is comparable")
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
	checkPanic("missing field name", func() {
		reflect.StructOf([]reflect.StructField{{Type: uint64Type}})
	})
	checkPanic("invalid field name", func() {
		reflect.StructOf([]reflect.StructField{{Name: "1field", Type: uint64Type}})
	})
	checkPanic("missing field type", func() {
		reflect.StructOf([]reflect.StructField{{Name: "Field"}})
	})
	checkPanic("duplicate field name", func() {
		reflect.StructOf([]reflect.StructField{
			{Name: "Field", Type: uint64Type},
			{Name: "Field", Type: uint64Type},
		})
	})
	checkPanic("different package paths", func() {
		reflect.StructOf([]reflect.StructField{
			{Name: "first", PkgPath: "one", Type: uint64Type},
			{Name: "second", PkgPath: "two", Type: uint64Type},
		})
	})
	checkPanic("long struct tag", func() {
		reflect.StructOf([]reflect.StructField{{
			Name: "Field",
			Type: uint64Type,
			Tag:  reflect.StructTag(strings.Repeat("x", 256)),
		}})
	})
	methodType := reflect.StructOf([]reflect.StructField{{
		Name:      "StructOfEmbeddedWithMethod",
		Type:      reflect.TypeOf(StructOfEmbeddedWithMethod{}),
		Anonymous: true,
	}})
	if methodType.NumMethod() != 1 {
		t.Fatalf("StructOf embedded method count = %d, want 1", methodType.NumMethod())
	}
	method, ok := methodType.MethodByName("Method")
	if !ok {
		t.Fatal("StructOf promoted method was not found")
	}
	methodValue := reflect.New(methodType).Elem()
	if got := method.Func.Call([]reflect.Value{methodValue})[0].Int(); got != 42 {
		t.Fatalf("StructOf Method.Func returned %d, want 42", got)
	}
	if got := methodValue.MethodByName("Method").Call(nil)[0].Int(); got != 42 {
		t.Fatalf("StructOf method value returned %d, want 42", got)
	}
	interfaceType := reflect.TypeOf((*structOfMethod)(nil)).Elem()
	if !methodType.Implements(interfaceType) {
		t.Fatal("StructOf type does not implement structOfMethod")
	}
	if got := methodValue.Interface().(structOfMethod).Method(); got != 42 {
		t.Fatalf("StructOf interface method returned %d, want 42", got)
	}

	pointerMethodType := reflect.StructOf([]reflect.StructField{{
		Name:      "StructOfEmbeddedPointer",
		Type:      reflect.TypeOf((*StructOfEmbeddedPointer)(nil)),
		Anonymous: true,
	}})
	pointerMethodValue := reflect.New(pointerMethodType).Elem()
	pointerMethodValue.Field(0).Set(reflect.ValueOf(&StructOfEmbeddedPointer{Value: 42}))
	if got := pointerMethodValue.Interface().(structOfMethod).Method(); got != 42 {
		t.Fatalf("StructOf pointer interface method returned %d, want 42", got)
	}
	pointerMethodValue.Interface().(structOfSetter).Set(43)
	if got := pointerMethodValue.Field(0).Elem().Field(0).Int(); got != 43 {
		t.Fatalf("StructOf pointer interface Set stored %d, want 43", got)
	}
	dynamicPointerType := reflect.PointerTo(pointerMethodType)
	if dynamicPointerType.NumMethod() != 2 {
		t.Fatalf("PointerTo(StructOf) method count = %d, want 2", dynamicPointerType.NumMethod())
	}
	if got := pointerMethodValue.Addr().Interface().(structOfMethod).Method(); got != 43 {
		t.Fatalf("PointerTo(StructOf) interface method returned %d, want 43", got)
	}
	interfaceMethodType := reflect.StructOf([]reflect.StructField{{
		Name:      "StructOfEmbeddedInterface",
		Type:      reflect.TypeOf((*StructOfEmbeddedInterface)(nil)).Elem(),
		Anonymous: true,
	}})
	interfaceMethodValue := reflect.New(interfaceMethodType).Elem()
	interfaceMethodValue.Field(0).Set(reflect.ValueOf(StructOfEmbeddedWithMethod{}))
	if got := interfaceMethodValue.Interface().(structOfMethod).Method(); got != 42 {
		t.Fatalf("StructOf embedded interface method returned %d, want 42", got)
	}
	abiType := reflect.StructOf([]reflect.StructField{{
		Name:      "StructOfEmbeddedABI",
		Type:      reflect.TypeOf(StructOfEmbeddedABI{}),
		Anonymous: true,
	}})
	abiValue := reflect.New(abiType).Elem()
	abiValue.Field(0).Field(0).SetInt(40)
	abi := abiValue.Interface().(structOfABI)
	a, b, pair, text := abi.Mix(1, 2, structOfPair{3, 4}, "five")
	if a != 1 || b != 42 || pair != (structOfPair{3, 4}) || text != "five" {
		t.Fatalf("StructOf mixed ABI method returned %d, %d, %v, %q", a, b, pair, text)
	}
	if got := abi.Sum(1, 2); got != 43 {
		t.Fatalf("StructOf variadic interface method returned %d, want 43", got)
	}
	sum := abiValue.MethodByName("Sum")
	if got := sum.CallSlice([]reflect.Value{reflect.ValueOf([]int{1, 2})})[0].Int(); got != 43 {
		t.Fatalf("StructOf variadic method value returned %d, want 43", got)
	}
	checkPanic("embedded type with methods and multiple fields", func() {
		reflect.StructOf([]reflect.StructField{
			{
				Name:      "StructOfEmbeddedWithMethod",
				Type:      reflect.TypeOf(StructOfEmbeddedWithMethod{}),
				Anonymous: true,
			},
			{Name: "Value", Type: uint64Type},
		})
	})
	checkPanic("map with uncomparable struct key", func() {
		reflect.MapOf(reflect.StructOf([]reflect.StructField{{
			Name: "Values",
			Type: reflect.TypeOf([]int(nil)),
		}}), uint64Type)
	})
}

func TestStructOfConcurrentConstruction(t *testing.T) {
	fields := []reflect.StructField{{
		Name: "Value",
		Type: reflect.TypeOf(int(0)),
		Tag:  "concurrent",
	}}
	const count = 8
	results := make(chan reflect.Type, count)
	for range count {
		go func() {
			results <- reflect.StructOf(fields)
		}()
	}

	want := <-results
	for range count - 1 {
		if got := <-results; got != want {
			t.Fatalf("concurrent StructOf returned %v, want %v", got, want)
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
