package makefunchelper

import "reflect"

func Make(typ reflect.Type, fn func([]reflect.Value) []reflect.Value) any {
	return reflect.MakeFunc(typ, fn).Interface()
}
