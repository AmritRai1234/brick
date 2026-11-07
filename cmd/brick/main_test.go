package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompanionCFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("/* c */\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("prog.brick")
	write("prog.c")
	write("prog.d/alpha.c")
	write("prog.d/beta.c")
	// Decoys that must never be linked.
	write("other.c")
	write("helper.c")
	write("prog.d/notes.txt")

	got := companionCFiles(filepath.Join(dir, "prog.brick"))
	want := []string{
		filepath.Join(dir, "prog.c"),
		filepath.Join(dir, "prog.d", "alpha.c"),
		filepath.Join(dir, "prog.d", "beta.c"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCompanionCFilesNoCompanions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "solo.brick"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := companionCFiles(filepath.Join(dir, "solo.brick")); len(got) != 0 {
		t.Errorf("expected no files, got %v", got)
	}
}

func TestFirstDiagnostic(t *testing.T) {
	cases := []struct{ src, want string }{
		{`function main() { print(1) }`, ""},
		{"function main() {\n    print((1 + 2)\n}\n", `3:1: expected ")", found "}"`},
		{"function main() {\n    var x: int = \"hi\"\n}\n", `2:5: cannot assign text to "int" (no implicit conversion)`},
	}
	for _, tc := range cases {
		if got := firstDiagnostic(tc.src); got != tc.want {
			t.Errorf("firstDiagnostic(%q) = %q, want %q", tc.src, got, tc.want)
		}
	}
}
