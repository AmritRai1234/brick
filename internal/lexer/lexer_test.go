package lexer

import (
	"testing"

	"brick/internal/token"
)

func TestBasicTokens(t *testing.T) {
	src := `var x = 1 + 2 * 3`
	want := []token.Type{
		token.VAR, token.IDENT, token.ASSIGN, token.INT,
		token.PLUS, token.INT, token.STAR, token.INT, token.EOF,
	}
	got := New(src).Tokens()
	if len(got) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Errorf("token %d: got %s, want %s", i, got[i].Type, w)
		}
	}
}

func TestOperators(t *testing.T) {
	src := `-> => == != <= >= && || &mut & !`
	types := []token.Type{
		token.ARROW, token.FATARROW, token.EQ, token.NEQ, token.LE, token.GE,
		token.AND, token.OR, token.AMPMUT, token.AMP, token.NOT, token.EOF,
	}
	got := New(src).Tokens()
	for i, w := range types {
		if got[i].Type != w {
			t.Errorf("token %d: got %s (%q), want %s", i, got[i].Type, got[i].Literal, w)
		}
	}
}

func TestStringEscapes(t *testing.T) {
	got := New(`"a\nb\t\"c\""`).Tokens()
	if got[0].Type != token.STRING {
		t.Fatalf("want STRING, got %s", got[0].Type)
	}
	if got[0].Literal != "a\nb\t\"c\"" {
		t.Errorf("got %q", got[0].Literal)
	}
}

func TestLineCol(t *testing.T) {
	got := New("var\nx").Tokens()
	if got[0].Line != 1 || got[0].Col != 1 {
		t.Errorf("var at %d:%d, want 1:1", got[0].Line, got[0].Col)
	}
	if got[1].Line != 2 || got[1].Col != 1 {
		t.Errorf("x at %d:%d, want 2:1", got[1].Line, got[1].Col)
	}
}

func TestCharLiterals(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`'a'`, "a"},
		{`'\n'`, "\n"},
		{`'\t'`, "\t"},
		{`'\''`, "'"},
		{`'\\'`, "\\"},
		{`'0'`, "0"},
	}
	for _, c := range cases {
		got := New(c.src).Tokens()
		if got[0].Type != token.CHAR {
			t.Errorf("%s: want CHAR, got %s", c.src, got[0].Type)
			continue
		}
		if got[0].Literal != c.want {
			t.Errorf("%s: literal %q, want %q", c.src, got[0].Literal, c.want)
		}
	}
}

func TestCharLiteralErrors(t *testing.T) {
	for _, src := range []string{`'ab'`, `'a`, `''`, `'\q'`} {
		got := New(src).Tokens()
		if got[0].Type != token.ILLEGAL {
			t.Errorf("%s: want ILLEGAL, got %s", src, got[0].Type)
		}
	}
}
