package wasmgc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestCompileManagedPointer(t *testing.T) {
	const source = `package main

type node struct {
	value int
	next *node
}

func main() {
	first := &node{value: 20}
	second := &node{value: 22}
	first.next = second
	value := &first.value
	*value += second.value
	println(first.value)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(type $type0 (struct",
		"(field (mut (ref null $type0)))",
		"(struct.new_default $type0)",
		"(drop (call $suspend))",
		"(call $printInt",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileGoroutineChannel(t *testing.T) {
	const source = `package main

type node struct {
	value int
}

func produce(ch chan int) {
	value := &node{value: 42}
	ch <- value.value
}

func main() {
	ch := make(chan int)
	go produce(ch)
	println(<-ch)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		`(import "env" "channelSend"`,
		`(import "env" "channelRecv"`,
		`(import "env" "scheduleTask"`,
		`(global $tasks0_head`,
		`(export "runTask")`,
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileManagedLoop(t *testing.T) {
	const source = `package main

type node struct {
	value int
	next *node
}

func main() {
	var head *node
	for i := 0; i < 10; i++ {
		head = &node{value: i, next: head}
	}
	total := 0
	for value := head; value != nil; value = value.next {
		total += value.value
	}
	println(total)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(loop $dispatch",
		"(ref.eq",
		"(br $dispatch)",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileManagedSlice(t *testing.T) {
	const source = `package main
func makeValues(count int) []int {
	values := make([]int, count)
	for i := 0; i < len(values); i++ {
		values[i] = i
	}
	return values
}
func main() {
	values := makeValues(8)[2:6]
	println(values[1], len(values), cap(values))
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(type $array0 (array (mut i32)))",
		"(array.new_default $array0",
		"(array.set $array0",
		"(array.get $array0",
		"(result (ref null $array0)) (result i32) (result i32) (result i32)",
		"(if (i32.lt_s",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileNilSlice(t *testing.T) {
	const source = `package main
func pass(values []int) []int { return values }
func main() {
	var values []int
	println(len(pass(values)), values == nil)
}
`
	wat := compileSource(t, source)
	if !strings.Contains(wat, "(type $array0 (array (mut i32)))") {
		t.Fatalf("slice type was not discovered from the function signature:\n%s", wat)
	}
	if !strings.Contains(wat, "(ref.is_null") {
		t.Fatalf("nil slice comparison was not lowered:\n%s", wat)
	}
}

func TestCompileSliceAppend(t *testing.T) {
	const source = `package main
func main() {
	values := make([]int, 1, 2)
	values[0] = 40
	values = append(values, 2)
	values = append(values[:1], values...)
	println(values[0] + values[1] + values[2])
	var bytes []byte
	bytes = append(bytes, "go"...)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(array.copy $array",
		"_append_cap i32",
		"(i32.mul",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompilePointerSlice(t *testing.T) {
	const source = `package main
type value struct { number int }
func main() {
	values := []*value{{number: 40}, nil}
	values[1] = &value{number: 2}
	values = append(values, values...)
	println(values[0].number + values[1].number)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(array (mut (ref null $type",
		"(struct.new $type",
		"(array.get $array",
		"(array.copy $array",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileSliceCopy(t *testing.T) {
	const source = `package main
type value struct { number int }
func main() {
	numbers := []int{1, 2, 3}
	other := make([]int, len(numbers))
	copy(other, numbers)
	bytes := make([]byte, 2)
	copy(bytes, "go")
	pointers := []*value{{number: 40}}
	copiedPointers := make([]*value, 1)
	copy(copiedPointers, pointers)
	println(other[0], bytes[0], copiedPointers[0].number)
}
`
	wat := compileSource(t, source)
	if count := strings.Count(wat, "(array.copy $array"); count != 3 {
		t.Fatalf("expected three array.copy instructions, got %d:\n%s", count, wat)
	}
}

func TestCompileStructSliceCopy(t *testing.T) {
	const source = `package main
type value struct { number int }
func main() {
	source := []value{{number: 1}, {number: 2}, {number: 3}}
	destination := make([]value, 3)
	slot := &destination[0]
	copy(destination, source)
	var nilDestination []value
	copy(nilDestination, source)
	copy(source[1:], source[:2])
	copy(source[:2], source[1:])
	println(slot.number, source[0].number)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"_copy_source (ref null $type",
		"_copy_destination (ref null $type",
		"_copy_backward",
		"_copy_forward",
		"(struct.new $type",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileStructEquality(t *testing.T) {
	const source = `package main
type node struct { number int }
type value struct {
	number int
	pointer *node
}
func main() {
	node := &node{number: 2}
	left := value{number: 40, pointer: node}
	right := value{number: 40, pointer: node}
	println(left == right, left != value{number: 41, pointer: node})
	println(struct{}{} == struct{}{})
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(ref.eq (struct.get $type",
		"(i32.eq (struct.get $type",
		"(i32.eqz (i32.and",
		"(local.set $t",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileSliceClear(t *testing.T) {
	const source = `package main
type value struct { number int }
func main() {
	numbers := []int{1, 2}
	clear(numbers)
	pointers := []*value{{number: 3}}
	clear(pointers)
	structs := []value{{number: 4}}
	slot := &structs[0]
	clear(structs)
	var nilStructs []value
	clear(nilStructs)
	println(numbers[0], pointers[0] == nil, slot.number)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(array.fill $array",
		"(func $clearStructArray",
		"(call $clearStructArray",
		"(struct.set $type",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileAnonymousPointerSlice(t *testing.T) {
	const source = `package main
func replace(values []*struct{ number int }) {
	values[0] = &struct{ number int }{number: 42}
}
func number(value *struct{ number int }) int {
	return value.number
}
func main() {
	values := []*struct{ number int }{{number: 1}}
	replace(values)
	println(number(values[0]))
}
`
	compileSource(t, source)
}

func TestCompileStructValue(t *testing.T) {
	const source = `package main
type node struct { number int }
type value struct { number int; pointer *node }
type converted struct { number int; pointer *node }
func makeValue() value {
	return value{number: 40, pointer: &node{number: 2}}
}
func update(input value) value {
	input.number++
	return input
}
func main() {
	original := makeValue()
	updated := update(original)
	other := converted(updated)
	println(original.number, updated.number, other.pointer.number)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(result (ref null $type",
		"(struct.new $type",
		"(struct.get $type",
		"(struct.set $type",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileZeroAndEmptyStructValues(t *testing.T) {
	const source = `package main
type value struct { number int }
func zero() value {
	var result value
	return result
}
func load(pointer *struct{}) struct{} {
	return *pointer
}
func store(pointer *struct{}) {
	*pointer = struct{}{}
}
func main() {
	empty := struct{}{}
	load(&empty)
	store(&empty)
	println(zero().number)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(struct.new_default $type",
		"(drop (ref.as_non_null",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileNilStructDereference(t *testing.T) {
	const source = `package main
type value struct{ number int }
func load() value {
	return *(*value)(nil)
}
func store() {
	*(*value)(nil) = value{}
}
func field() {
	pointer := &(*value)(nil).number
	*pointer = 1
}
func main() {
	load()
	store()
	field()
}
`
	wat, err := compileSourceError(t, source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wat, "(ref.as_non_null (ref.null") {
		t.Fatalf("output does not contain a constant nil check:\n%s", wat)
	}
}

func TestCompileStructGlobal(t *testing.T) {
	const source = `package main
type value struct { number int }
var global = value{number: 40}
func use(pointer *value) {
	println(pointer.number)
}
func main() {
	snapshot := global
	pointer := &global
	global = value{number: 42}
	go use(&global)
	println(snapshot.number, pointer.number)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(global $global",
		"(struct.new_default $type",
		"(global.get $global",
		"(struct.set $type",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileStructSlice(t *testing.T) {
	const source = `package main
type value struct { number int }
func main() {
	values := make([]value, 2)
	pointer := &values[0]
	values[0] = value{number: 40}
	values[1] = values[0]
	values[1].number += 2
	println(pointer.number, values[1].number)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(array (mut (ref null $type",
		"(array.get $array",
		"(struct.new_default $type",
		"(array.set $array",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileStructSliceAppend(t *testing.T) {
	const source = `package main
type value struct { number int }
func main() {
	values := make([]value, 1, 2)
	values[0].number = 40
	extended := values[:2]
	spare := &extended[1]
	values = append(values, value{number: 2})
	values = append(values[:1], values...)
	println(spare.number, values[0].number)
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"_append_destination (ref null $type",
		"_append_added_backward",
		"_append_added_forward",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
	if strings.Contains(wat, "_append_values") {
		t.Fatalf("struct append allocated a temporary snapshot array:\n%s", wat)
	}
}

func TestCompileEmptyStructSlice(t *testing.T) {
	const source = `package main
func main() {
	values := make([]struct{}, 2)
	first := &values[0]
	second := &values[1]
	*first = *second
	println(len(values))
}
`
	compileSource(t, source)
}

func TestCompileManagedString(t *testing.T) {
	const source = `package main
var message = "wasmgc!"
func middle(value string) string { return value[2:7] }
func main() {
	value := middle(message)
	println(len(value), value[0], value == "smgc!")
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(type $array0 (array (mut i8)))",
		"(array.new_fixed $array0",
		"(func $stringEqual",
		"(array.get_u $array0",
		"(global.set $global",
		"(result (ref null $array0)) (result i32) (result i32)",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestCompileStringEdgeCases(t *testing.T) {
	const source = `package main
func increment(value byte) byte { return value + 1 }
func main() {
	number := 300
	println(len(""[0:0]), "abc"[1], increment(255), byte(number))
}
`
	wat := compileSource(t, source)
	if strings.Contains(wat, "$nil:string_len") || strings.Contains(wat, "$\"abc\":string_len") {
		t.Fatalf("string constant length used an undeclared local:\n%s", wat)
	}
	if !strings.Contains(wat, "(i32.const 255)") {
		t.Fatalf("uint8 narrowing was not emitted:\n%s", wat)
	}
}

func TestCompileDirectClosure(t *testing.T) {
	const source = `package main
type value struct { number int }
func main() {
	item := &value{number: 40}
	add := func(delta int) int { return item.number + delta }
	println(add(2))
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(field (mut (ref null $type",
		"(call $fn",
		"(param $item_base",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestSpawnedClosureUsesManagedTask(t *testing.T) {
	const source = `package main
type value struct { number int }
func main() {
	value := &value{number: 42}
	go func() { println(value.number) }()
}
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(type $task0 (struct",
		"(global $tasks0_head (mut (ref null $task0))",
		"(global $tasks0_tail (mut (ref null $task0))",
		"(local.set $task0_new (struct.new $task0",
		"(export \"runTask\")",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func TestRejectCapturedArray(t *testing.T) {
	const source = `package main
func main() {
	values := [2]int{40, 2}
	add := func() int { return values[0] + values[1] }
	println(add())
}
`
	_, err := compileSourceError(t, source)
	if err == nil || !strings.Contains(err.Error(), "captured array variables are not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectUnsupportedPrograms(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "unsupported global",
			source: `package main
var value float64
func main() {}
`,
			message: "unsupported global type value: float64",
		},
		{
			name: "integer width",
			source: `package main
func main() {
	var value int8
	println(value)
}
`,
			message: "unsupported value type: int8",
		},
		{
			name: "global address comparison",
			source: `package main
var value int
func main() { println(&value == &value) }
`,
			message: "global address comparisons are not supported",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileSourceError(t, test.source)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCompileInit(t *testing.T) {
	const source = `package main
func init() { println(1) }
func main() { println(2) }
`
	wat := compileSource(t, source)
	if strings.Count(wat, "(call $printInt") != 2 {
		t.Fatalf("package init was not compiled:\n%s", wat)
	}
}

func TestCompileGlobals(t *testing.T) {
	const source = `package main
type node struct { value int }
type nodePointer *node
var scalar = 3
var root nodePointer = nodePointer(&node{value: 39})
func init() { scalar += root.value }
func main() { println(scalar) }
`
	wat := compileSource(t, source)
	for _, expected := range []string{
		"(global $global",
		"(mut (ref null $type0))",
		"(global.set $global",
		"(global.get $global",
	} {
		if !strings.Contains(wat, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, wat)
		}
	}
}

func compileSource(t *testing.T, source string) string {
	t.Helper()
	wat, err := compileSourceError(t, source)
	if err != nil {
		t.Fatal(err)
	}
	return wat
}

func compileSourceError(t *testing.T, source string) (string, error) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	files := []*ast.File{file}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Scopes:     make(map[ast.Node]*types.Scope),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Instances:  make(map[*ast.Ident]types.Instance),
	}
	typesPkg, err := (&types.Config{}).Check("main", fset, files, info)
	if err != nil {
		t.Fatal(err)
	}
	prog := ssa.NewProgram(fset, 0)
	pkg := prog.CreatePackage(typesPkg, files, info, true)

	wat, err := Compile(Program{Main: pkg})
	return wat, err
}
