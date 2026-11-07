package parser

import (
	"testing"

	"brick/internal/ast"
)

func TestParseFunction(t *testing.T) {
	prog, err := Parse(`function square(n: int) -> int { return n * n }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("want 1 statement, got %d", len(prog.Stmts))
	}
	fd, ok := prog.Stmts[0].(*ast.FuncDecl)
	if !ok {
		t.Fatalf("want FuncDecl, got %T", prog.Stmts[0])
	}
	if fd.Name != "square" || len(fd.Params) != 1 || fd.ReturnType.Name != "int" {
		t.Errorf("unexpected decl: %+v", fd)
	}
	ret, ok := fd.Body.Stmts[0].(*ast.ReturnStmt)
	if !ok || ret.Value == nil {
		t.Fatalf("want return with value")
	}
}

func TestPrecedence(t *testing.T) {
	prog, err := Parse(`function main() { var x = 1 + 2 * 3 }`)
	if err != nil {
		t.Fatal(err)
	}
	fd := prog.Stmts[0].(*ast.FuncDecl)
	vs := fd.Body.Stmts[0].(*ast.VarStmt)
	bin, ok := vs.Value.(*ast.BinaryExpr)
	if !ok || bin.Op != "+" {
		t.Fatalf("top op should be +, got %T", vs.Value)
	}
	if _, ok := bin.Right.(*ast.BinaryExpr); !ok {
		t.Errorf("right side should be a * expression")
	}
}

func TestStructLiteralInCondition(t *testing.T) {
	// The `{` after `ready` must be the if-body, not a struct literal.
	_, err := Parse(`function main() { var ready = true if ready { print(1) } }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyntaxErrorHasPosition(t *testing.T) {
	_, err := Parse("function main( {")
	if err == nil {
		t.Fatal("want an error")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("want *ParseError, got %T", err)
	}
	if pe.Line == 0 {
		t.Errorf("error should carry a line number")
	}
}

func TestParseEnum(t *testing.T) {
	prog, err := Parse(`enum Shape { Circle(float), Rect(float, float), Point }`)
	if err != nil {
		t.Fatal(err)
	}
	ed, ok := prog.Stmts[0].(*ast.EnumDecl)
	if !ok {
		t.Fatalf("want EnumDecl, got %T", prog.Stmts[0])
	}
	if len(ed.Variants) != 3 || len(ed.Variants[1].Fields) != 2 || len(ed.Variants[2].Fields) != 0 {
		t.Errorf("unexpected variants: %+v", ed.Variants)
	}
}

func TestParseMatch(t *testing.T) {
	prog, err := Parse(`function main() {
		match x {
			Color.Red => print(1)
			Shape.Circle(r) => { print(r) }
			_ => print(0)
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	fd := prog.Stmts[0].(*ast.FuncDecl)
	ms, ok := fd.Body.Stmts[0].(*ast.MatchStmt)
	if !ok {
		t.Fatalf("want MatchStmt, got %T", fd.Body.Stmts[0])
	}
	if len(ms.Arms) != 3 {
		t.Fatalf("want 3 arms, got %d", len(ms.Arms))
	}
	if !ms.Arms[2].Pattern.IsWild {
		t.Errorf("third arm should be a wildcard")
	}
	if len(ms.Arms[1].Pattern.Binds) != 1 || ms.Arms[1].Pattern.Binds[0] != "r" {
		t.Errorf("second arm should bind r, got %+v", ms.Arms[1].Pattern)
	}
}

func TestParseIndexAndListType(t *testing.T) {
	prog, err := Parse(`function f(xs: List(int)) -> int { return xs[0] }`)
	if err != nil {
		t.Fatal(err)
	}
	fd := prog.Stmts[0].(*ast.FuncDecl)
	if fd.Params[0].Type.Name != "List" || fd.Params[0].Type.Elem == nil || fd.Params[0].Type.Elem.Name != "int" {
		t.Errorf("param type should be List(int), got %s", fd.Params[0].Type)
	}
	ret := fd.Body.Stmts[0].(*ast.ReturnStmt)
	if _, ok := ret.Value.(*ast.IndexExpr); !ok {
		t.Errorf("return value should be an index expression, got %T", ret.Value)
	}
}

func TestParseArena(t *testing.T) {
	prog, err := Parse(`function main() { arena { var x = 1 } }`)
	if err != nil {
		t.Fatal(err)
	}
	fd := prog.Stmts[0].(*ast.FuncDecl)
	ar, ok := fd.Body.Stmts[0].(*ast.ArenaStmt)
	if !ok {
		t.Fatalf("want ArenaStmt, got %T", fd.Body.Stmts[0])
	}
	if ar.Body == nil || len(ar.Body.Stmts) != 1 {
		t.Fatalf("arena body not parsed: %+v", ar.Body)
	}
}

func TestParseBreakContinue(t *testing.T) {
	prog, err := Parse(`function main() { while true { break continue } }`)
	if err != nil {
		t.Fatal(err)
	}
	fd := prog.Stmts[0].(*ast.FuncDecl)
	w := fd.Body.Stmts[0].(*ast.WhileStmt)
	if _, ok := w.Body.Stmts[0].(*ast.BreakStmt); !ok {
		t.Fatalf("want BreakStmt, got %T", w.Body.Stmts[0])
	}
	if _, ok := w.Body.Stmts[1].(*ast.ContinueStmt); !ok {
		t.Fatalf("want ContinueStmt, got %T", w.Body.Stmts[1])
	}
}

func TestParseFor(t *testing.T) {
	prog, err := Parse(`function main() { for x in xs { print(x) } }`)
	if err != nil {
		t.Fatal(err)
	}
	fd := prog.Stmts[0].(*ast.FuncDecl)
	fs, ok := fd.Body.Stmts[0].(*ast.ForStmt)
	if !ok {
		t.Fatalf("want ForStmt, got %T", fd.Body.Stmts[0])
	}
	if fs.Var != "x" {
		t.Errorf("loop var = %q, want x", fs.Var)
	}
	if _, ok := fs.Iter.(*ast.Ident); !ok {
		t.Errorf("iter should be an ident, got %T", fs.Iter)
	}
	if fs.Body == nil || len(fs.Body.Stmts) != 1 {
		t.Fatalf("body not parsed: %+v", fs.Body)
	}
}
