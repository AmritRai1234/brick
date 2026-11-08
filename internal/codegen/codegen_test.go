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

func TestFileAndArgsCodegen(t *testing.T) {
	out := gen(t, `
	function main() {
		var argv = args()
		var data = read_file(argv[1])
		write_file("out.txt", data)
		eprint(data.len)
	}`)
	for _, want := range []string{
		"int main(int brick_argc, char **brick_argv)",
		"brick_saved_argc = brick_argc;",
		"brick_args()",
		"brick_read_file((*(const char**)brick_list_at(&(argv), 1)))",
		"brick_write_file(\"out.txt\", data)",
		"fprintf(stderr, ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestNoArgsPlumbingWhenUnused(t *testing.T) {
	out := gen(t, `function main() { print(1) }`)
	if strings.Contains(out, "brick_saved_argc") {
		t.Errorf("args plumbing should not appear when args() is unused\n%s", out)
	}
	if !strings.Contains(out, "int main(void)") {
		t.Errorf("main should be int main(void) when args() is unused\n%s", out)
	}
}

func TestArenaCodegen(t *testing.T) {
	out := gen(t, `
	function main() {
		arena {
			var xs = List(int)
			xs.push(1)
		}
	}`)
	for _, want := range []string{
		"BrickArena brick_arena_1;",
		"brick_arena_enter(&brick_arena_1);",
		"brick_arena_leave(&brick_arena_1);",
		"static inline void *brick_alloc(size_t n)",
		"static inline void *brick_resize(",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestLoopJumpCodegen(t *testing.T) {
	out := gen(t, `
	function main() {
		var i = 0
		while i < 10 {
			i = i + 1
			if i == 3 { continue }
			if i == 8 { break }
		}
	}`)
	for _, want := range []string{
		"brick_loop_1:",
		"while (",
		"goto brick_loop_1;",     // continue -> loop start
		"goto brick_loop_1_end;", // break -> loop end
		"brick_loop_1_end: ;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestNoLabelsWithoutJumps(t *testing.T) {
	out := gen(t, `function main() { var i = 0 while i < 3 { i = i + 1 } }`)
	if strings.Contains(out, "brick_loop_") {
		t.Errorf("no jump should emit no labels\n%s", out)
	}
}

func TestMapCodegen(t *testing.T) {
	out := gen(t, `
	function main() {
		var m = Map(text, int)
		m.set("a", 1)
		print(m.get("a"))
		print(m.has("a"))
		print(m.len)
	}`)
	for _, want := range []string{
		"static inline BrickMap brick_map_new(int64_t elem)",
		"brick_map_new(sizeof(int64_t))",
		"BRICK_MAP_SET(int64_t, (m), (\"a\"), (1))",
		"brick_map_get_must(&(m), \"a\")",
		"brick_map_has(&(m), \"a\")",
		"((m).len)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestBuilderAndToTextCodegen(t *testing.T) {
	out := gen(t, `
	function main() {
		var b = Builder()
		b.add("x")
		print(b.build())
		print(b.len)
		print(to_text(3))
	}`)
	for _, want := range []string{
		"static inline BrickBuilder brick_builder_new(void)",
		"brick_builder_add(&(b), \"x\")",
		"brick_builder_build(&(b))",
		"brick_int_to_text(3)",
		"((int64_t)(b).len)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestConstCodegenInlines(t *testing.T) {
	out := gen(t, `
	const MAX: int = 10
	function main() { print(MAX) }`)
	if !strings.Contains(out, "(long long)(10)") {
		t.Errorf("const should be inlined as a literal\n%s", out)
	}
	if strings.Contains(out, "MAX") {
		t.Errorf("const name should not appear in the output\n%s", out)
	}
}

func TestForCodegen(t *testing.T) {
	out := gen(t, `
	function main() {
		var xs = List(int)
		var total = 0
		for x in xs { total = total + x }
		print(total)
	}`)
	for _, want := range []string{
		"brick_iter_1",
		"brick_i_1",
		"while (brick_i_1 < brick_iter_1.len)",
		"brick_list_at(&(brick_iter_1), brick_i_1)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestForContinueLabel(t *testing.T) {
	out := gen(t, `
	function main() {
		var xs = List(int)
		for x in xs { continue }
	}`)
	if !strings.Contains(out, "brick_cont_") {
		t.Errorf("continue should emit a continue label\n%s", out)
	}
}

func TestCharLiteralCodegen(t *testing.T) {
	out := gen(t, `function main() { print('a') }`)
	if !strings.Contains(out, "(long long)(97)") {
		t.Errorf("'a' should lower to the int 97\n%s", out)
	}
}

func TestToTextCodegenByType(t *testing.T) {
	out := gen(t, `function main() { print(to_text(2)) print(to_text(1.5)) print(to_text(true)) }`)
	for _, want := range []string{
		"brick_int_to_text(2)",
		"brick_float_to_text(1.5)",
		"brick_bool_to_text(true)",
		"brick_float_to_text(double value)",
		"brick_bool_to_text(bool value)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestStringControlBytesEscaped(t *testing.T) {
	out := gen(t, `function main() { print("a\0b") }`)
	// The NUL must be a fixed-width octal escape, never a raw control byte
	// (a raw NUL in the emitted C literal warns under -Wall).
	if !strings.Contains(out, `"a\000b"`) {
		t.Errorf("expected octal-escaped NUL, got:\n%s", out)
	}
	if strings.ContainsRune(out, '\x00') {
		t.Errorf("emitted C contains a raw NUL byte:\n%s", out)
	}
}

func TestBytesRuntimeOnlyWhenUsed(t *testing.T) {
	plain := gen(t, `function main() { print(1) }`)
	if strings.Contains(plain, "BrickBytes") {
		t.Error("bytes runtime should not be emitted for a program that does not use bytes")
	}
	withBytes := gen(t, `function main() {
    var b = to_bytes("hi")
    print(b.len)
    print(b[0])
    print(to_text(b))
    print(b == to_bytes("hi"))
}`)
	for _, want := range []string{
		"typedef struct\n{\n    char   *data;\n    int64_t len;\n} BrickBytes;",
		"BrickBytes b = brick_to_bytes(\"hi\");",
		"brick_bytes_at(b, 0)",
		"brick_bytes_to_text(b)",
		"brick_bytes_eq(b, brick_to_bytes(\"hi\"))",
	} {
		if !strings.Contains(withBytes, want) {
			t.Errorf("output missing %q\n%s", want, withBytes)
		}
	}
}

func TestExitCodegen(t *testing.T) {
	out := gen(t, `function main() { exit(2) }`)
	if !strings.Contains(out, "exit((int)(2))") {
		t.Errorf("expected a direct exit() call, got:\n%s", out)
	}
}

func TestSystemCodegen(t *testing.T) {
	withSystem := gen(t, `function main() { print(system(args())) }`)
	for _, want := range []string{
		"static inline int64_t brick_run(BrickList argv)",
		"execvp(vec[0], vec);",
		"brick_run(brick_args())",
	} {
		if !strings.Contains(withSystem, want) {
			t.Errorf("output missing %q\n%s", want, withSystem)
		}
	}
	// A program that never calls system carries no fork/exec code.
	plain := gen(t, `function main() { print(1) }`)
	if strings.Contains(plain, "brick_run") {
		t.Errorf("system runtime should be omitted:\n%s", plain)
	}
}

func TestGetenvCodegen(t *testing.T) {
	out := gen(t, `function main() { print(getenv("HOME")) }`)
	for _, want := range []string{
		"return value != NULL ? value : \"\";",
		"brick_getenv(\"HOME\")",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if plain := gen(t, `function main() { print(1) }`); strings.Contains(plain, "brick_getenv") {
		t.Errorf("getenv runtime should be omitted:\n%s", plain)
	}
}

func TestReadDirCodegen(t *testing.T) {
	out := gen(t, `function main() { var xs = read_dir(".") print(xs.len) print(is_dir(".")) }`)
	for _, want := range []string{
		"static inline BrickList brick_read_dir(const char *path)",
		"qsort(list.data, (size_t)list.len, sizeof(name), brick_name_compare);",
		"static inline bool brick_is_dir(const char *path)",
		"brick_read_dir(\".\")",
		"brick_is_dir(\".\")",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if plain := gen(t, `function main() { print(1) }`); strings.Contains(plain, "brick_read_dir") || strings.Contains(plain, "brick_is_dir") {
		t.Errorf("directory runtime should be omitted:\n%s", plain)
	}
}

func TestBuilderAddIntFastPath(t *testing.T) {
	out := gen(t, `
	function main() {
		var b = Builder()
		var i = 0
		while i < 3 {
			b.add(to_text(i))
			b.add(",")
			i = i + 1
		}
		print(b.build())
	}`)
	if !strings.Contains(out, "brick_builder_add_int(&(b), i)") {
		t.Errorf("builder.add(to_text(int)) should append the integer directly\n%s", out)
	}
	if strings.Contains(out, "brick_builder_add(&(b), brick_int_to_text(") {
		t.Errorf("builder.add(to_text(int)) should not build intermediate text\n%s", out)
	}
	// Only ints take the fast path: everything else still goes through add().
	for _, src := range []string{
		`function main() { var b = Builder() b.add(to_text(1.5)) print(b.build()) }`,
		`function main() { var b = Builder() b.add(to_text(true)) print(b.build()) }`,
		`function main() { var b = Builder() b.add("x") print(b.build()) }`,
	} {
		other := gen(t, src)
		if strings.Contains(other, "brick_builder_add_int") && !strings.Contains(other, "brick_builder_add(&(") {
			t.Errorf("non-int add should not use the int fast path\n%s", other)
		}
	}
}

func TestIntToTextDoesNotFormat(t *testing.T) {
	out := gen(t, `function main() { print(to_text(42)) }`)
	helper := out[strings.Index(out, "brick_int_to_text(int64_t value)"):]
	helper = helper[:strings.Index(helper, "\n}")]
	if strings.Contains(helper, "snprintf") {
		t.Errorf("brick_int_to_text should convert by hand, not via snprintf:\n%s", helper)
	}
	for _, want := range []string{"char     digits[24]", "magnitude % 10u", "brick_alloc(len + 1)"} {
		if !strings.Contains(helper, want) {
			t.Errorf("brick_int_to_text missing %q:\n%s", want, helper)
		}
	}
}

func TestBuilderAddIsSingleStrlen(t *testing.T) {
	out := gen(t, `function main() { var b = Builder() b.add("x") print(b.build()) }`)
	helper := out[strings.Index(out, "static inline void brick_builder_add("):]
	helper = helper[:strings.Index(helper, "\n}")]
	if got := strings.Count(helper, "strlen(text)"); got != 1 {
		t.Errorf("brick_builder_add should call strlen once, got %d:\n%s", got, helper)
	}
	if !strings.Contains(out, "static inline void brick_builder_reserve(BrickBuilder *builder, size_t need)") {
		t.Errorf("builder growth should be shared through brick_builder_reserve\n%s", out)
	}
}

// Names that C owns are legal Brick names; the emitter writes "bx_" in front of
// them so the generated C cannot clash with the C library or the runtime.
func TestReservedNamesAreRenamed(t *testing.T) {
	out := gen(t, `
	struct long { printf: int }
	enum size_t { Small(int), Big(int) }
	function double(n: int) -> int { return n * 2 }
	function main() {
		var strlen = 3
		var p = long{ printf: strlen }
		print(double(p.printf))
		var s = size_t.Small(1)
		match s { size_t.Small(x) => print(x), size_t.Big(y) => print(y) }
	}`)
	for _, want := range []string{
		"typedef struct bx_long bx_long;",
		"typedef struct bx_size_t bx_size_t;",
		"struct bx_long {",
		"    int64_t bx_printf;",
		"static int64_t bx_double(int64_t n);",
		"int64_t bx_strlen = 3;",
		"(bx_long){ .bx_printf = bx_strlen }",
		"bx_double((p).bx_printf)",
		"enum { bx_size_t_Small = 0, bx_size_t_Big = 1 };",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

// A program that uses none of those names must generate exactly the C it did
// before the rename existed: no "bx_" anywhere.
func TestOrdinaryNamesAreUntouched(t *testing.T) {
	out := gen(t, `struct P { x: int } function main() { var p = P{ x: 1 } print(p.x) }`)
	// "bx_" only ever appears through cname; the runtime prelude has none.
	if strings.Contains(out, "bx_") {
		t.Errorf("unexpected renaming in ordinary program:\n%s", out)
	}
}
