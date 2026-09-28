package reflect_test

import (
	"io"
	"reflect"
	"reflect/internal/makefunchelper"
	"runtime"
	"testing"
)

func TestMakeFuncAllocations(t *testing.T) {
	for _, test := range callAllocationCases() {
		t.Run(test.name, func(t *testing.T) {
			value := reflect.MakeFunc(test.fn.Type(), func(in []reflect.Value) []reflect.Value { return in })
			max := test.max + 1
			if test.name == "empty" {
				max = 0
			}
			if got := testing.AllocsPerRun(100, func() {
				callAllocationResults = value.Call(test.args)
			}); got > max {
				t.Fatalf("MakeFunc Call allocated %v times, want at most %v", got, max)
			}
		})
	}
	value := reflect.MakeFunc(reflect.TypeOf((func())(nil)), func(in []reflect.Value) []reflect.Value { return in })
	if got := testing.AllocsPerRun(100, value.Interface().(func())); got != 0 {
		t.Fatalf("MakeFunc empty function allocated %v times, want 0", got)
	}
	value = reflect.MakeFunc(reflect.TypeOf((func(int) int)(nil)), func(in []reflect.Value) []reflect.Value { return in })
	fn := value.Interface().(func(int) int)
	if got := testing.AllocsPerRun(100, func() {
		if fn(42) != 42 {
			t.Fatal("MakeFunc returned the wrong value")
		}
	}); got > 1 {
		t.Fatalf("MakeFunc scalar function allocated %v times, want at most 1", got)
	}
}

func BenchmarkMakeFuncCall(b *testing.B) {
	for _, test := range callAllocationCases() {
		b.Run(test.name, func(b *testing.B) {
			value := reflect.MakeFunc(test.fn.Type(), func(in []reflect.Value) []reflect.Value { return in })
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				callAllocationResults = value.Call(test.args)
			}
		})
	}
}

func TestMakeFuncRetainedArguments(t *testing.T) {
	type payload struct {
		Pointer *int
		Padding [1100]byte
	}
	var saved []reflect.Value
	value := reflect.MakeFunc(reflect.TypeOf((func(payload) payload)(nil)), func(in []reflect.Value) []reflect.Value {
		saved = append(saved, in[0])
		runtime.GC()
		return in
	})
	fn := value.Interface().(func(payload) payload)
	input := payload{Pointer: new(int)}
	*input.Pointer = 17
	input.Padding[0] = 1
	first := fn(input)
	input.Padding[0] = 2
	input.Pointer = new(int)
	*input.Pointer = 23
	second := fn(input)
	input.Padding[0] = 3
	third := value.Call([]reflect.Value{reflect.ValueOf(&input).Elem()})[0]
	input.Padding[0] = 4
	input.Pointer = nil
	runtime.GC()
	for i, want := range []payload{first, second, third.Interface().(payload)} {
		got := saved[i].Interface().(payload)
		if got != want || got.Padding[0] != byte(i+1) || *got.Pointer != []int{17, 23, 23}[i] {
			t.Fatalf("retained argument %d was changed", i)
		}
		if saved[i].CanAddr() || saved[i].CanSet() {
			t.Fatalf("retained argument %d is addressable", i)
		}
	}
}

func TestMakeFunc(t *testing.T) {
	type pair [2]uintptr
	typ := reflect.TypeOf(func(byte, int, byte, pair, byte, float32, byte) (byte, int, byte, pair, byte, float32, byte) {
		return 0, 0, 0, pair{}, 0, 0, 0
	})
	value := reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
		return in
	})
	fn := value.Interface().(func(byte, int, byte, pair, byte, float32, byte) (byte, int, byte, pair, byte, float32, byte))
	b, c, d, e, f, g, h := fn(10, 20, 30, pair{40, 50}, 60, 70, 80)
	if b != 10 || c != 20 || d != 30 || e != (pair{40, 50}) || f != 60 || g != 70 || h != 80 {
		t.Fatalf("MakeFunc returned %d, %d, %d, %v, %d, %g, %d", b, c, d, e, f, g, h)
	}

}

func TestMakeFuncAcrossPackages(t *testing.T) {
	value := makefunchelper.Make(
		reflect.TypeOf((func(int) int)(nil)),
		func(args []reflect.Value) []reflect.Value {
			return []reflect.Value{reflect.ValueOf(int(args[0].Int()) + 1)}
		},
	)
	if got := value.(func(int) int)(41); got != 42 {
		t.Fatalf("MakeFunc result returned %d, want 42", got)
	}
}

func TestMakeFuncCall(t *testing.T) {
	typ := reflect.TypeOf(func(int) int { return 0 })
	value := reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
		if in[0].CanAddr() {
			t.Error("MakeFunc argument is addressable")
		}
		return []reflect.Value{reflect.ValueOf(int(in[0].Int()) + 1)}
	})
	out := value.Call([]reflect.Value{reflect.ValueOf(41)})
	if got := int(out[0].Int()); got != 42 {
		t.Fatalf("Call returned %d, want 42", got)
	}
}

func TestMakeFuncVariadic(t *testing.T) {
	typ := reflect.TypeOf(func(int, ...int) []int { return nil })
	value := reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
		return in[1:2]
	})
	fn := value.Interface().(func(int, ...int) []int)
	got := fn(1, 2, 3)
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("MakeFunc returned %v, want [2 3]", got)
	}
}

func TestMakeFuncAssignableResult(t *testing.T) {
	typ := reflect.TypeOf(func() error { return nil })
	value := reflect.MakeFunc(typ, func([]reflect.Value) []reflect.Value {
		return []reflect.Value{reflect.ValueOf(io.EOF)}
	})
	if got := value.Interface().(func() error)(); got != io.EOF {
		t.Fatalf("MakeFunc returned %v, want %v", got, io.EOF)
	}
}

func TestMakeFuncZeroSized(t *testing.T) {
	type empty struct{}
	typ := reflect.TypeOf(func(empty) empty { return empty{} })
	value := reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
		return in
	})
	if got := value.Interface().(func(empty) empty)(empty{}); got != (empty{}) {
		t.Fatalf("MakeFunc returned %#v", got)
	}
}

func TestMakeFuncIndirectAggregate(t *testing.T) {
	type large [1100]byte
	typ := reflect.TypeOf(func(large) large { return large{} })
	value := reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
		result := in[0].Interface().(large)
		result[0]++
		result[len(result)-1]++
		return []reflect.Value{reflect.ValueOf(result)}
	})
	var input large
	input[0] = 1
	input[len(input)-1] = 2
	got := value.Interface().(func(large) large)(input)
	if got[0] != 2 || got[len(got)-1] != 3 {
		t.Fatalf("result endpoints = %d, %d; want 2, 3", got[0], got[len(got)-1])
	}
}

func TestMakeFuncDynamicType(t *testing.T) {
	type input string
	type output float64
	typ := reflect.FuncOf(
		[]reflect.Type{reflect.TypeOf(input(""))},
		[]reflect.Type{reflect.TypeOf(output(0))},
		false,
	)
	value := reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
		if got := in[0].Interface().(input); got != "gopher" {
			t.Fatalf("argument = %q, want %q", got, "gopher")
		}
		return []reflect.Value{reflect.ValueOf(output(3.14))}
	})
	out := value.Call([]reflect.Value{reflect.ValueOf(input("gopher"))})
	if got := out[0].Interface().(output); got != 3.14 {
		t.Fatalf("Call returned %v, want 3.14", got)
	}
}

func TestMakeFuncPanics(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		fn()
	}

	mustPanic("non-function type", func() {
		reflect.MakeFunc(reflect.TypeOf(0), func([]reflect.Value) []reflect.Value { return nil })
	})
	mustPanic("wrong result count", func() {
		value := reflect.MakeFunc(reflect.TypeOf(func() int { return 0 }), func([]reflect.Value) []reflect.Value {
			return nil
		})
		value.Call(nil)
	})
	mustPanic("zero result", func() {
		value := reflect.MakeFunc(reflect.TypeOf(func() int { return 0 }), func([]reflect.Value) []reflect.Value {
			return []reflect.Value{{}}
		})
		value.Call(nil)
	})
	mustPanic("wrong result type", func() {
		value := reflect.MakeFunc(reflect.TypeOf(func() int { return 0 }), func([]reflect.Value) []reflect.Value {
			return []reflect.Value{reflect.ValueOf("bad")}
		})
		value.Call(nil)
	})
}
