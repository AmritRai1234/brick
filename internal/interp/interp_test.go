package interp

import (
	"bytes"
	"strings"
	"testing"

	"brick/internal/checker"
	"brick/internal/parser"
)

// runInterp parses, checks and interprets src, returning stdout.
func runInterp(t *testing.T, src string, args ...string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ck, err := checker.Check(prog)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	var out, errb bytes.Buffer
	err = Run(prog, ck, Options{Args: args, Stdout: &out, Stderr: &errb})
	return out.String(), err
}

func mustRun(t *testing.T, src string, want string) {
	t.Helper()
	got, err := runInterp(t, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimRight(got, "\n") != strings.TrimRight(want, "\n") {
		t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestArithmetic(t *testing.T) {
	src := `
	function square(n: int) -> int { return n * n }
	function main() {
		var total = 0
		var i = 1
		while i <= 5 {
			total = total + square(i)
			i = i + 1
		}
		print(total)   // 55
		print(7 % 3)   // 1
		print(7 / 2)   // 3
	}`
	mustRun(t, src, "55\n1\n3")
}

func TestStringsAndLists(t *testing.T) {
	src := `
	function main() {
		var s = "hello"
		print(s.len)          // 5
		print(s[0])           // 104
		print(s + " world")   // hello world
		print(s == "nope")    // false

		var xs = List(int)
		xs.push(10)
		xs.push(20)
		print(xs.len)         // 2
		xs[0] = 99
		print(xs[0])          // 99
	}`
	mustRun(t, src, "5\n104\nhello world\nfalse\n2\n99")
}

func TestEnumAndMatch(t *testing.T) {
	src := `
	enum Result { Ok(int), Err(text) }
	function divide(a: int, b: int) -> Result {
		if b == 0 { return Result.Err("division by zero") }
		return Result.Ok(a / b)
	}
	function main() {
		match divide(10, 2) {
			Result.Ok(n) => print(n)
			Result.Err(m) => print(m)
		}
		match divide(1, 0) {
			Result.Ok(n) => print(n)
			Result.Err(m) => print(m)
		}
	}`
	mustRun(t, src, "5\ndivision by zero")
}

func TestStructsAndBorrows(t *testing.T) {
	src := `
	struct Point { x: int, y: int }
	function move(p: &mut Point) {
		p.x = p.x + 1
		p.y = p.y + 1
	}
	function main() {
		var a = Point{ x: 1, y: 2 }
		move(&mut a)
		print(a.x)   // 2
		print(a.y)   // 3
	}`
	mustRun(t, src, "2\n3")
}

func TestShapeVariants(t *testing.T) {
	src := `
	enum Shape { Circle(float), Rect(float, float) }
	function area(s: Shape) -> float {
		match s {
			Shape.Circle(r) => { return 3.14159 * r * r }
			Shape.Rect(w, h) => { return w * h }
		}
		return 0.0
	}
	function main() {
		print(area(Shape.Circle(1.0)))    // 3.14159
		print(area(Shape.Rect(3.0, 4.0))) // 12
	}`
	mustRun(t, src, "3.14159\n12")
}

func TestArgs(t *testing.T) {
	src := `
	function main() {
		var argv = args()
		print(argv.len)
		if argv.len > 1 { print(argv[1]) }
	}`
	got, err := runInterp(t, src, "prog", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimRight(got, "\n") != "2\nhello" {
		t.Fatalf("got %q", got)
	}
}

func TestDivisionByZero(t *testing.T) {
	if _, err := runInterp(t, `function main() { print(1 / 0) }`); err == nil {
		t.Fatal("want a runtime error for division by zero")
	}
}

func TestIndexOutOfRange(t *testing.T) {
	src := `
	function main() {
		var xs = List(int)
		print(xs[3])
	}`
	if _, err := runInterp(t, src); err == nil {
		t.Fatal("want a runtime error for out-of-range index")
	}
}
