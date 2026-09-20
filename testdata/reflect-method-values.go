package main

import "reflect"

type methodTarget int

func (v methodTarget) Keep() int  { return int(v) + 1 }
func (v methodTarget) Other() int { return int(v) + 2 }

var lookupValue = reflect.Value.MethodByName
var lookupType = reflect.Type.MethodByName
var indexedValue = reflect.Value.Method
var boundLookup = reflect.ValueOf(methodTarget(41)).MethodByName
var enumerateValue = reflect.Value.Methods

func main() {
	value := reflect.ValueOf(methodTarget(41))
	if lookupValue(value, "Keep").Call(nil)[0].Int() != 42 {
		panic("value method expression failed")
	}
	method, ok := lookupType(value.Type(), "Other")
	if !ok || method.Func.Call([]reflect.Value{value})[0].Int() != 43 {
		panic("type method expression failed")
	}
	if indexedValue(value, 1).Call(nil)[0].Int() != 43 {
		panic("indexed method expression failed")
	}
	if boundLookup("Keep").Call(nil)[0].Int() != 42 {
		panic("initialized bound method failed")
	}
	count := 0
	for metadata, method := range enumerateValue(value) {
		if method.Call(nil)[0].Int() != int64(42+metadata.Index) {
			panic("method iterator expression failed")
		}
		count++
	}
	if count != 2 {
		panic("method count changed")
	}
	println("initialized reflected method values passed")
}
