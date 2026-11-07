package lexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The self-hosted scanner (selfhost/tokens.brick) is the twin of this package.
// In Brick a text is a plain const char *, so `.len` and slice(text, ...) both
// cost a strlen of the whole string. Reading the source length once per
// character made the self-hosted compiler quadratic (15 s for a 6,400-function
// file); it now caches the length in Lexer.srcLen and builds token literals from
// bytes. This test keeps that shape from regressing.
func TestSelfHostedScannerStaysLinear(t *testing.T) {
	path := filepath.Join("..", "..", "selfhost", "tokens.brick")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(src)

	if !strings.Contains(text, "srcLen: int") {
		t.Error("Lexer must cache the source length in srcLen: text.len is a strlen")
	}
	if strings.Contains(text, "slice(l.src") {
		t.Error("scanner must not slice the source: slice(text, ...) is a strlen of the whole file")
	}
	if n := strings.Count(text, ".src.len"); n != 0 {
		t.Errorf("scanner reads src.len %d time(s); use the cached srcLen inside scans", n)
	}
}
