package checker

import (
	"testing"

	"brick/internal/ast"
	"brick/internal/parser"
)

func check(t *testing.T, src string) (*Checker, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return Check(prog)
}

func TestInfersInt(t *testing.T) {
	ck, err := check(t, `function main() { var x = 2 + 3 print(x) }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ck.Types) == 0 {
		t.Fatal("expected inferred types")
	}
}

func TestRejectMixedArithmetic(t *testing.T) {
	_, err := check(t, `function main() { var x = 1 + "two" }`)
	if err == nil {
		t.Fatal("want an error for int + text")
	}
}

func TestRejectUndefined(t *testing.T) {
	_, err := check(t, `function main() { print(nope) }`)
	if err == nil {
		t.Fatal("want an error for undefined name")
	}
}

func TestRejectNoImplicitConversion(t *testing.T) {
	_, err := check(t, `function main() { var x: float = 3 }`)
	if err == nil {
		t.Fatal("want an error for int assigned to float")
	}
}

func TestBorrowType(t *testing.T) {
	src := `
	struct P { x: int }
	function bump(p: &mut P) { p.x = p.x + 1 }
	function main() { var a = P{ x: 1 } bump(&mut a) }`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMissingFieldRejected(t *testing.T) {
	src := `
	struct P { x: int, y: int }
	function main() { var a = P{ x: 1 } }`
	if _, err := check(t, src); err == nil {
		t.Fatal("want an error for a missing field")
	}
}

func TestTopLevelStatementRejected(t *testing.T) {
	if _, err := check(t, `var x = 1`); err == nil {
		t.Fatal("want an error for a top-level statement")
	}
	_ = ast.Program{}
}

func TestEnumAndMatch(t *testing.T) {
	src := `
	enum Result { Ok(int), Err(text) }
	function f(b: int) -> Result {
		if b == 0 { return Result.Err("zero") }
		return Result.Ok(b)
	}
	function main() {
		match f(1) {
			Result.Ok(n) => print(n)
			Result.Err(m) => print(m)
		}
	}`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMatchExhaustiveness(t *testing.T) {
	src := `
	enum Color { Red, Green, Blue }
	function main() {
		var c = Color.Red
		match c { Color.Red => print(1) Color.Green => print(2) }
	}`
	if _, err := check(t, src); err == nil {
		t.Fatal("want an error for a non-exhaustive match")
	}
}

func TestMatchWildcardIsExhaustive(t *testing.T) {
	src := `
	enum Color { Red, Green, Blue }
	function main() {
		var c = Color.Red
		match c { Color.Red => print(1) _ => print(0) }
	}`
	if _, err := check(t, src); err != nil {
		t.Fatalf("wildcard should satisfy exhaustiveness: %v", err)
	}
}

func TestMatchBindingArity(t *testing.T) {
	src := `
	enum R { Ok(int), Err(text) }
	function main() {
		var r = R.Ok(1)
		match r { R.Ok(a, b) => print(a) R.Err(m) => print(m) }
	}`
	if _, err := check(t, src); err == nil {
		t.Fatal("want an error for wrong binding count")
	}
}

func TestEnumVariantArityAtConstruction(t *testing.T) {
	src := `
	enum R { Ok(int), Err(text) }
	function main() { var r = R.Ok(1, 2) }`
	if _, err := check(t, src); err == nil {
		t.Fatal("want an error for too many constructor arguments")
	}
}

func TestListPushAndIndex(t *testing.T) {
	src := `
	function main() {
		var xs = List(int)
		xs.push(1)
		xs.push(2)
		var n = xs.len
		var x = xs[0]
		xs[1] = 9
		print(n + x)
	}`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListPushTypeMismatch(t *testing.T) {
	_, err := check(t, `function main() { var xs = List(int) xs.push("no") }`)
	if err == nil {
		t.Fatal("want an error pushing text into List(int)")
	}
}

func TestListOfStructs(t *testing.T) {
	src := `
	struct P { x: int }
	function main() {
		var ps = List(P)
		ps.push(P{ x: 1 })
		print(ps[0].x)
	}`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTextConcatAndLen(t *testing.T) {
	src := `
	function main() {
		var s = "ab"
		var t = s + "cd"
		var n = t.len
		var c = s[0]
		var b = t == "abcd"
		print(n + c)
		if b { print(1) } else { print(0) }
	}`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCannotAssignToTextIndex(t *testing.T) {
	_, err := check(t, `function main() { var s = "hi" s[0] = 5 }`)
	if err == nil {
		t.Fatal("want an error assigning to a text index")
	}
}

func TestFileAndArgsBuiltins(t *testing.T) {
	src := `
	function main() {
		var argv = args()
		if argv.len < 2 { eprint("usage") return }
		var data = read_file(argv[1])
		write_file("out.txt", data)
		print(data.len)
	}`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteFileArity(t *testing.T) {
	if _, err := check(t, `function main() { write_file("x") }`); err == nil {
		t.Fatal("want an error for write_file with one argument")
	}
}

func TestReadFileArgType(t *testing.T) {
	if _, err := check(t, `function main() { var s = read_file(3) }`); err == nil {
		t.Fatal("want an error for a non-text path")
	}
}

func TestArenaScope(t *testing.T) {
	src := `
	function main() {
		arena {
			var xs = List(int)
			xs.push(1)
		}
	}`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArenaValueDoesNotEscape(t *testing.T) {
	src := `
	function main() {
		arena { var xs = List(int) }
		print(xs.len)
	}`
	if _, err := check(t, src); err == nil {
		t.Fatal("want an error: an arena-local name must not escape the block")
	}
}

func TestBreakOutsideLoop(t *testing.T) {
	if _, err := check(t, `function main() { break }`); err == nil {
		t.Fatal("want an error for break outside a loop")
	}
}

func TestContinueOutsideLoop(t *testing.T) {
	if _, err := check(t, `function main() { continue }`); err == nil {
		t.Fatal("want an error for continue outside a loop")
	}
}

func TestBreakInLoopOK(t *testing.T) {
	src := `function main() { var i = 0 while true { if i == 1 { break } i = i + 1 } }`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMapTypes(t *testing.T) {
	src := `
	function main() {
		var m = Map(text, int)
		m.set("a", 1)
		print(m.len)
		print(m.get("a"))
		print(m.has("a"))
	}`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMapValueTypeChecked(t *testing.T) {
	src := `function main() { var m = Map(text, int) m.set("a", "oops") }`
	if _, err := check(t, src); err == nil {
		t.Fatal("want an error for a value of the wrong type")
	}
}

func TestMapKeyMustBeText(t *testing.T) {
	if _, err := check(t, `function main() { var m = Map(int, int) }`); err == nil {
		t.Fatal("want an error for non-text map keys")
	}
}

func TestBuilderAndToText(t *testing.T) {
	src := `
	function main() {
		var b = Builder()
		b.add("x")
		b.add(to_text(7))
		var s = b.build()
		print(s)
		print(b.len)
	}`
	if _, err := check(t, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuilderAddNeedsText(t *testing.T) {
	if _, err := check(t, `function main() { var b = Builder() b.add(3) }`); err == nil {
		t.Fatal("want an error: add needs text (use to_text)")
	}
}

func TestToTextNeedsInt(t *testing.T) {
	if _, err := check(t, `function main() { print(to_text("x")) }`); err == nil {
		t.Fatal("want an error: to_text needs an int")
	}
}
