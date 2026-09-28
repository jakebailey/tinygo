package reflect_test

import (
	"reflect"
	"testing"
)

type methodValueTarget struct {
	Base int
}

func (v methodValueTarget) Add(value int) int {
	return v.Base + value
}

func (v *methodValueTarget) Set(value int) {
	v.Base = value
}

func (v methodValueTarget) Sum(values ...int) int {
	total := v.Base
	for _, value := range values {
		total += value
	}
	return total
}

type embeddedMethodTarget struct {
	methodValueTarget
}

func TestTypeMethodFunc(t *testing.T) {
	typ := reflect.TypeOf(methodValueTarget{})
	method, ok := typ.MethodByName("Add")
	if !ok {
		t.Fatal("Add method was not found")
	}

	out := method.Func.Call([]reflect.Value{
		reflect.ValueOf(methodValueTarget{Base: 40}),
		reflect.ValueOf(2),
	})
	if got := int(out[0].Int()); got != 42 {
		t.Fatalf("Method.Func returned %d, want 42", got)
	}
}

func TestInterfaceTypeMethodFunc(t *testing.T) {
	typ := reflect.TypeOf((*interface{ Add(int) int })(nil)).Elem()
	method, ok := typ.MethodByName("Add")
	if !ok {
		t.Fatal("Add method was not found")
	}
	if method.Func.IsValid() {
		t.Fatalf("interface Method.Func is %v, want zero Value", method.Func)
	}
}

func TestValueMethod(t *testing.T) {
	value := reflect.ValueOf(methodValueTarget{Base: 40}).MethodByName("Add")
	if !value.IsValid() {
		t.Fatal("Add method value was not found")
	}
	out := value.Call([]reflect.Value{reflect.ValueOf(2)})
	if got := int(out[0].Int()); got != 42 {
		t.Fatalf("method value returned %d, want 42", got)
	}
	fn := value.Interface().(func(int) int)
	if got := fn(3); got != 43 {
		t.Fatalf("method function returned %d, want 43", got)
	}
}

func TestValuePointerMethod(t *testing.T) {
	target := &methodValueTarget{}
	method := reflect.ValueOf(target).MethodByName("Set")
	method.Call([]reflect.Value{reflect.ValueOf(42)})
	if target.Base != 42 {
		t.Fatalf("Set stored %d, want 42", target.Base)
	}
}

func TestValueVariadicMethod(t *testing.T) {
	method := reflect.ValueOf(methodValueTarget{Base: 40}).MethodByName("Sum")
	out := method.Call([]reflect.Value{
		reflect.ValueOf(1),
		reflect.ValueOf(2),
	})
	if got := int(out[0].Int()); got != 43 {
		t.Fatalf("variadic method returned %d, want 43", got)
	}
	out = method.CallSlice([]reflect.Value{
		reflect.ValueOf([]int{1, 2}),
	})
	if got := int(out[0].Int()); got != 43 {
		t.Fatalf("variadic CallSlice returned %d, want 43", got)
	}
}

func TestValuePromotedMethod(t *testing.T) {
	target := embeddedMethodTarget{methodValueTarget{Base: 40}}
	out := reflect.ValueOf(target).MethodByName("Add").Call([]reflect.Value{reflect.ValueOf(2)})
	if got := int(out[0].Int()); got != 42 {
		t.Fatalf("promoted method returned %d, want 42", got)
	}
}

func TestValueInterfaceMethod(t *testing.T) {
	var target interface{ Add(int) int } = methodValueTarget{Base: 40}
	value := reflect.ValueOf(&target).Elem()
	out := value.MethodByName("Add").Call([]reflect.Value{reflect.ValueOf(2)})
	if got := int(out[0].Int()); got != 42 {
		t.Fatalf("interface method returned %d, want 42", got)
	}
}

func TestValueMethodByNameMissing(t *testing.T) {
	if value := reflect.ValueOf(methodValueTarget{}).MethodByName("Missing"); value.IsValid() {
		t.Fatalf("MethodByName returned %v for a missing method", value)
	}
}
