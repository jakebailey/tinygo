package main

import "reflect"

type number int

func (v number) String() string {
	return "number"
}

type function func() int

func (v function) String() string {
	return "function"
}

var numberMethod = reflect.ValueOf(number(23)).Method(0)
var functionMethod = reflect.ValueOf(function(nil)).Method(0)

func main() {
	if got := numberMethod.Call(nil)[0].String(); got != "number" {
		panic("initialized number method returned " + got)
	}
	if got := functionMethod.Call(nil)[0].String(); got != "function" {
		panic("initialized function method returned " + got)
	}
	println("initialized method metadata passed")
}
