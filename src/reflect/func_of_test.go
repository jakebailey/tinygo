package reflect_test

import (
	"reflect"
	"testing"
)

type funcOfNamed int

func TestFuncOfType(t *testing.T) {
	tests := []struct {
		in       []reflect.Type
		out      []reflect.Type
		variadic bool
		want     reflect.Type
	}{
		{want: reflect.TypeOf((func())(nil))},
		{
			in:   []reflect.Type{reflect.TypeOf(funcOfNamed(0))},
			want: reflect.TypeOf((func(funcOfNamed))(nil)),
		},
		{
			in:       []reflect.Type{reflect.TypeOf([]int(nil))},
			variadic: true,
			want:     reflect.TypeOf((func(...int))(nil)),
		},
		{
			in:   []reflect.Type{reflect.TypeOf(0)},
			out:  []reflect.Type{reflect.TypeOf(false), reflect.TypeOf("")},
			want: reflect.TypeOf((func(int) (bool, string))(nil)),
		},
	}

	for _, test := range tests {
		got := reflect.FuncOf(test.in, test.out, test.variadic)
		if got != test.want {
			t.Errorf("FuncOf(%v, %v, %v) = %v, want %v", test.in, test.out, test.variadic, got, test.want)
		}
		if again := reflect.FuncOf(test.in, test.out, test.variadic); again != got {
			t.Errorf("second FuncOf(%v, %v, %v) = %v, want identical %v", test.in, test.out, test.variadic, again, got)
		}
		if got.Comparable() {
			t.Errorf("FuncOf(%v, %v, %v) is comparable", test.in, test.out, test.variadic)
		}
	}
}

func TestFuncOfDynamicType(t *testing.T) {
	parameterType := reflect.PointerTo(reflect.TypeOf(funcOfNamed(0)))
	got := reflect.FuncOf(
		[]reflect.Type{parameterType, reflect.TypeOf([]string(nil))},
		[]reflect.Type{reflect.TypeOf(false)},
		true,
	)
	if got.String() != "func(*reflect_test.funcOfNamed, ...string) bool" {
		t.Fatalf("FuncOf returned %v", got)
	}
	if got.In(0) != parameterType || got.In(1) != reflect.TypeOf([]string(nil)) || got.Out(0) != reflect.TypeOf(false) {
		t.Fatalf("FuncOf returned incorrect parameter or result types")
	}
	if !got.IsVariadic() {
		t.Fatal("FuncOf returned a non-variadic type")
	}
	if reflect.PointerTo(got).Elem() != got {
		t.Fatal("PointerTo(FuncOf(...)).Elem() did not preserve type identity")
	}

	in := make([]reflect.Type, 51)
	for i := range in {
		in[i] = reflect.TypeOf(0)
	}
	if got := reflect.FuncOf(in, nil, false); got.NumIn() != len(in) {
		t.Fatalf("FuncOf with 51 inputs has %d inputs", got.NumIn())
	}
}

func TestFuncOfResultCount(t *testing.T) {
	result := reflect.TypeOf(funcOfNamed(0))
	out := make([]reflect.Type, 128)
	for i := range out {
		out[i] = result
	}
	for _, test := range []struct {
		in       []reflect.Type
		out      []reflect.Type
		variadic bool
	}{
		{out: out},
		{in: []reflect.Type{reflect.TypeOf([]int(nil))}, out: out[:127], variadic: true},
	} {
		typ := reflect.FuncOf(test.in, test.out, test.variadic)
		if typ.NumIn() != len(test.in) || typ.NumOut() != len(test.out) || typ.IsVariadic() != test.variadic {
			t.Fatalf("FuncOf has %d inputs, %d outputs, variadic %v", typ.NumIn(), typ.NumOut(), typ.IsVariadic())
		}
		for i := range test.out {
			if got := typ.Out(i); got != result {
				t.Fatalf("Out(%d) = %v, want %v", i, got, result)
			}
		}
		if again := reflect.FuncOf(test.in, test.out, test.variadic); again != typ {
			t.Fatal("FuncOf did not preserve type identity")
		}
		if reflect.PointerTo(typ).Elem() != typ {
			t.Fatal("PointerTo(FuncOf(...)) lost the function type")
		}
	}
}

func TestFuncOfPanics(t *testing.T) {
	assertPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		fn()
	}

	assertPanic("variadic without inputs", func() {
		reflect.FuncOf(nil, nil, true)
	})
	assertPanic("variadic with non-slice input", func() {
		reflect.FuncOf([]reflect.Type{reflect.TypeOf(0)}, nil, true)
	})

	in := make([]reflect.Type, 129)
	for i := range in {
		in[i] = reflect.TypeOf(0)
	}
	assertPanic("too many arguments", func() {
		reflect.FuncOf(in, nil, false)
	})

	out := make([]reflect.Type, 129)
	for i := range out {
		out[i] = reflect.TypeOf(0)
	}
	assertPanic("too many results", func() {
		reflect.FuncOf(nil, out, false)
	})
}
