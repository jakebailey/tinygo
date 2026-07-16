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
		`(import "env" "spawn0"`,
		`(export "goroutine0")`,
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

func TestRejectUnsupportedPrograms(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "unsupported global",
			source: `package main
var value string
func main() {}
`,
			message: "unsupported global type value: string",
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
