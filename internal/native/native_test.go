package native

import (
	"strings"
	"testing"

	"brick/internal/checker"
	"brick/internal/parser"
)

func gen(t *testing.T, src string) (string, error) {
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

func mustGen(t *testing.T, src string) string {
	t.Helper()
	out, err := gen(t, src)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return out
}

func TestMultiplyAndMain(t *testing.T) {
	out := mustGen(t, `function square(n: int) -> int { return n * n }
function main() { print(square(5)) }`)
	for _, want := range []string{
		".globl main",
		".globl square",
		"imulq %rcx, %rax",
		"call printf",
		"pushq %rbp",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("assembly missing %q\n%s", want, out)
		}
	}
}

func TestComparisonsAndBranches(t *testing.T) {
	out := mustGen(t, `function main() { var x = 3 if x <= 5 { print(1) } while x > 0 { x = x - 1 } }`)
	for _, want := range []string{"setle", "setg", "je ", "jmp "} {
		if !strings.Contains(out, want) {
			t.Errorf("assembly missing %q\n%s", want, out)
		}
	}
}

func TestShortCircuit(t *testing.T) {
	out := mustGen(t, `function main() { var a = true var b = false print(a && b) print(a || b) }`)
	if strings.Count(out, "jmp ") < 2 {
		t.Errorf("expected short-circuit jumps\n%s", out)
	}
}

func TestUnsupportedStructs(t *testing.T) {
	_, err := gen(t, `struct P { x: int }
function main() { var p = P{ x: 1 } }`)
	if err == nil {
		t.Fatal("expected an UnsupportedError for structs")
	}
	if _, ok := err.(*UnsupportedError); !ok {
		t.Fatalf("want *UnsupportedError, got %T: %v", err, err)
	}
}

func TestUnsupportedFloat(t *testing.T) {
	_, err := gen(t, `function main() { print(1.5) }`)
	if _, ok := err.(*UnsupportedError); !ok {
		t.Fatalf("want *UnsupportedError for float, got %T: %v", err, err)
	}
}
