package codegen

import (
	"strings"
	"testing"

	"brick/internal/checker"
	"brick/internal/parser"
)

func gen(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ck, err := checker.Check(prog)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return Generate(prog, ck)
}

func TestVarAndPrint(t *testing.T) {
	out := gen(t, `function main() { var x = 2 + 3 print(x) }`)
	for _, want := range []string{
		"int64_t x = (2 + 3);",
		`printf("%lld\n", (long long)(x))`,
		"int main(void)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestStructAndBorrow(t *testing.T) {
	out := gen(t, `
	struct P { x: int, y: int }
	function move(p: &mut P) { p.x = p.x + 1 }
	function main() { var a = P{ x: 1, y: 2 } move(&mut a) }`)
	for _, want := range []string{
		"struct P {",
		"static void move(P* p)",
		"(p)->x = ((p)->x + 1);",
		"move((&(a)));",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestPrintFormats(t *testing.T) {
	out := gen(t, `function main() { print(1.5) print(true) print("hi") }`)
	if !strings.Contains(out, `printf("%g\n", (double)(1.5))`) {
		t.Errorf("float printing wrong:\n%s", out)
	}
	if !strings.Contains(out, `printf("%s\n", (true) ? "true" : "false")`) {
		t.Errorf("bool printing wrong:\n%s", out)
	}
	if !strings.Contains(out, `printf("%s\n", ("hi"))`) {
		t.Errorf("text printing wrong:\n%s", out)
	}
}

func TestEnumCodegen(t *testing.T) {
	out := gen(t, `
	enum Result { Ok(int), Err(text) }
	function main() {
		var r = Result.Ok(5)
		match r {
			Result.Ok(n) => print(n)
			Result.Err(m) => print(m)
		}
	}`)
	for _, want := range []string{
		"enum { Result_Ok = 0, Result_Err = 1 };",
		"struct Result {",
		"union {",
		"(Result){ .tag = 0, .u.Ok = { .payload_0 = 5 } }",
		"switch (scrutinee_1.tag)",
		"case 0: {",
		"int64_t n = scrutinee_1.u.Ok.payload_0;",
		"break;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestEnumWithoutPayload(t *testing.T) {
	out := gen(t, `
	enum Color { Red, Green, Blue }
	function main() {
		var c = Color.Green
		match c { Color.Red => print(1) Color.Green => print(2) Color.Blue => print(3) }
	}`)
	if !strings.Contains(out, "struct Color { int tag; };") {
		t.Errorf("payload-free enum should be a bare tag struct\n%s", out)
	}
	if !strings.Contains(out, "(Color){ .tag = 1 }") {
		t.Errorf("variant construction missing\n%s", out)
	}
}

func TestListCodegen(t *testing.T) {
	out := gen(t, `
	function main() {
		var xs = List(int)
		xs.push(7)
		print(xs[0])
		print(xs.len)
	}`)
	for _, want := range []string{
		"#define BRICK_LIST_INITIAL_CAPACITY 8",
		"} BrickList;",
		"brick_list_new(sizeof(int64_t))",
		"BRICK_PUSH(int64_t, (xs), (7))",
		"brick_list_at(&(xs), 0)",
		"((xs).len)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestTextCodegen(t *testing.T) {
	out := gen(t, `
	function main() {
		var s = "ab"
		var t = s + "cd"
		print(t == "abcd")
		print(s.len)
	}`)
	for _, want := range []string{
		"brick_text_concat(s, \"cd\")",
		"(strcmp(t, \"abcd\") == 0)",
		"((int64_t)strlen(s))",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}
