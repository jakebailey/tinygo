package reflect_test

import (
	"fmt"
	"reflect"
	"testing"
)

func TestValueCall(t *testing.T) {
	offset := 3
	fn := func(a int, b string) (int, string) {
		return a + offset, b + "!"
	}
	out := reflect.ValueOf(fn).Call([]reflect.Value{
		reflect.ValueOf(4),
		reflect.ValueOf("go"),
	})
	if got := int(out[0].Int()); got != 7 {
		t.Fatalf("first result = %d, want 7", got)
	}
	if got := out[1].String(); got != "go!" {
		t.Fatalf("second result = %q, want %q", got, "go!")
	}
	if out[0].CanAddr() || out[0].CanSet() {
		t.Fatal("Call result is addressable or settable")
	}
}

func TestValueCallNamedFunction(t *testing.T) {
	type named func(int) int
	var fn named = func(value int) int {
		return value + 1
	}
	out := reflect.ValueOf(fn).Call([]reflect.Value{reflect.ValueOf(41)})
	if got := int(out[0].Int()); got != 42 {
		t.Fatalf("result = %d, want 42", got)
	}
}

func TestValueCallAggregate(t *testing.T) {
	type pair struct {
		Number int
		Text   string
	}
	fn := func(value pair) pair {
		value.Number++
		value.Text += "!"
		return value
	}
	out := reflect.ValueOf(fn).Call([]reflect.Value{reflect.ValueOf(pair{41, "go"})})
	if got := out[0].Interface().(pair); got != (pair{42, "go!"}) {
		t.Fatalf("result = %#v, want %#v", got, pair{42, "go!"})
	}
}

func TestValueCallZeroSized(t *testing.T) {
	type empty struct{}
	fn := func(empty) empty {
		return empty{}
	}
	out := reflect.ValueOf(fn).Call([]reflect.Value{reflect.ValueOf(empty{})})
	if len(out) != 1 || out[0].Type() != reflect.TypeOf(empty{}) {
		t.Fatalf("Call returned %#v, want one empty result", out)
	}
}

func TestValueCallMixedABI(t *testing.T) {
	type pair [2]uintptr
	fn := func(b byte, c int, d byte, e pair, f byte, g float32, h byte) (byte, int, byte, pair, byte, float32, byte) {
		return b, c, d, e, f, g, h
	}
	out := reflect.ValueOf(fn).Call([]reflect.Value{
		reflect.ValueOf(byte(10)),
		reflect.ValueOf(20),
		reflect.ValueOf(byte(30)),
		reflect.ValueOf(pair{40, 50}),
		reflect.ValueOf(byte(60)),
		reflect.ValueOf(float32(70)),
		reflect.ValueOf(byte(80)),
	})
	if len(out) != 7 {
		t.Fatalf("Call returned %d values, want 7", len(out))
	}
	if byte(out[0].Uint()) != 10 ||
		int(out[1].Int()) != 20 ||
		byte(out[2].Uint()) != 30 ||
		out[3].Interface().(pair) != (pair{40, 50}) ||
		byte(out[4].Uint()) != 60 ||
		float32(out[5].Float()) != 70 ||
		byte(out[6].Uint()) != 80 {
		t.Fatalf("Call returned unexpected mixed results")
	}
}

func TestValueCallInterface(t *testing.T) {
	fn := func(value any) any {
		return struct{ Value any }{value}
	}
	out := reflect.ValueOf(fn).Call([]reflect.Value{reflect.ValueOf(42)})
	got := out[0].Interface().(struct{ Value any })
	if got.Value != 42 {
		t.Fatalf("result = %#v, want 42", got.Value)
	}
}

func TestValueCallInterfaceConversion(t *testing.T) {
	type reader interface {
		Read([]byte) (int, error)
	}
	fn := func(value reader) reader {
		return value
	}
	var value reader
	out := reflect.ValueOf(fn).Call([]reflect.Value{
		reflect.ValueOf(&value).Elem(),
	})
	if len(out) != 1 || !out[0].IsNil() || out[0].Type() != reflect.TypeOf((*reader)(nil)).Elem() {
		t.Fatalf("Call returned %#v, want a nil reader", out)
	}
}

func TestValueCallVariadic(t *testing.T) {
	fn := func(prefix string, values ...int) int {
		total := len(prefix)
		for _, value := range values {
			total += value
		}
		return total
	}
	value := reflect.ValueOf(fn)
	out := value.Call([]reflect.Value{
		reflect.ValueOf("go"),
		reflect.ValueOf(3),
		reflect.ValueOf(4),
	})
	if got := int(out[0].Int()); got != 9 {
		t.Fatalf("Call result = %d, want 9", got)
	}
	out = value.CallSlice([]reflect.Value{
		reflect.ValueOf("go"),
		reflect.ValueOf([]int{3, 4}),
	})
	if got := int(out[0].Int()); got != 9 {
		t.Fatalf("CallSlice result = %d, want 9", got)
	}
}

func TestValueCallImportedFunction(t *testing.T) {
	out := reflect.ValueOf(fmt.Sprintf).Call([]reflect.Value{
		reflect.ValueOf("%s %d"),
		reflect.ValueOf("value"),
		reflect.ValueOf(42),
	})
	if got := out[0].String(); got != "value 42" {
		t.Fatalf("Sprintf result = %q, want %q", got, "value 42")
	}
}

func TestValueCallIndirectAggregate(t *testing.T) {
	type large [1100]byte
	fn := func(value large) large {
		value[0]++
		value[len(value)-1]++
		return value
	}
	var input large
	input[0] = 1
	input[len(input)-1] = 2
	out := reflect.ValueOf(fn).Call([]reflect.Value{reflect.ValueOf(input)})
	got := out[0].Interface().(large)
	if got[0] != 2 || got[len(got)-1] != 3 {
		t.Fatalf("result endpoints = %d, %d; want 2, 3", got[0], got[len(got)-1])
	}
}

func TestValueCallPanics(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		fn()
	}

	mustPanic("non-function", func() {
		reflect.ValueOf(1).Call(nil)
	})
	mustPanic("nil function", func() {
		var fn func()
		reflect.ValueOf(fn).Call(nil)
	})
	mustPanic("wrong argument count", func() {
		reflect.ValueOf(func(int) {}).Call(nil)
	})
	mustPanic("wrong argument type", func() {
		reflect.ValueOf(func(int) {}).Call([]reflect.Value{reflect.ValueOf("x")})
	})
	mustPanic("CallSlice on non-variadic function", func() {
		reflect.ValueOf(func([]int) {}).CallSlice([]reflect.Value{reflect.ValueOf([]int{})})
	})
	mustPanic("called function", func() {
		reflect.ValueOf(func() { panic("called") }).Call(nil)
	})
}
