package loader

import (
	"os"
	"path/filepath"
	"testing"

	"brick/internal/ast"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func countFuncs(prog *ast.Program) int {
	n := 0
	for _, s := range prog.Stmts {
		if _, ok := s.(*ast.FuncDecl); ok {
			n++
		}
	}
	return n
}

func TestLoadMerges(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lib.brick", `function helper() -> int { return 1 }`)
	write(t, dir, "main.brick", "import \"lib\"\nfunction main() { print(helper()) }")

	prog, err := Load(filepath.Join(dir, "main.brick"))
	if err != nil {
		t.Fatal(err)
	}
	if got := countFuncs(prog); got != 2 {
		t.Fatalf("want 2 functions (helper + main), got %d", got)
	}
	for _, s := range prog.Stmts {
		if _, ok := s.(*ast.ImportStmt); ok {
			t.Fatal("import statements should be stripped")
		}
	}
}

func TestLoadDeduplicates(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.brick", `function shared() -> int { return 7 }`)
	write(t, dir, "b.brick", "import \"d\"\nfunction bee() -> int { return shared() }")
	write(t, dir, "main.brick", "import \"b\"\nimport \"d\"\nfunction main() { print(bee()) }")

	prog, err := Load(filepath.Join(dir, "main.brick"))
	if err != nil {
		t.Fatal(err)
	}
	// main, bee, shared -- shared must appear once even though reached twice.
	if got := countFuncs(prog); got != 3 {
		t.Fatalf("want 3 functions, got %d", got)
	}
}

func TestLoadCycle(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.brick", "import \"b\"\nfunction main() { print(1) }")
	write(t, dir, "b.brick", "import \"a\"\nfunction b() { print(2) }")
	if _, err := Load(filepath.Join(dir, "a.brick")); err == nil {
		t.Fatal("want an import cycle error")
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "main.brick", "import \"nope\"\nfunction main() { print(1) }")
	if _, err := Load(filepath.Join(dir, "main.brick")); err == nil {
		t.Fatal("want an error for a missing import")
	}
}
