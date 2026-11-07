package format

import "testing"

func TestNormalizesIndent(t *testing.T) {
	in := "function main() {\n\t\tvar x = 1\n   if x == 1 {\nprint(x)\n}\n}\n"
	want := "function main() {\n    var x = 1\n    if x == 1 {\n        print(x)\n    }\n}\n"
	if got := Source(in); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestPreservesComments(t *testing.T) {
	in := "// header\nfunction main() {\n    print(1) // trailing\n}\n"
	if got := Source(in); got != in {
		t.Fatalf("comments not preserved:\n%q", got)
	}
}

func TestBracesInStringsAndComments(t *testing.T) {
	in := "function main() {\n    print(\"a { b } c\")\n    // } } }\n}\n"
	if got := Source(in); got != in {
		t.Fatalf("braces in strings/comments miscounted:\n%q", got)
	}
}

func TestClosingBraceElse(t *testing.T) {
	in := "function main() {\n    if true {\n        print(1)\n    } else {\n        print(2)\n    }\n}\n"
	if got := Source(in); got != in {
		t.Fatalf("} else handling wrong:\n%q", got)
	}
}

func TestIdempotent(t *testing.T) {
	in := "  function  main ( )  {\n\t\tprint( 1 )\n }\n"
	once := Source(in)
	twice := Source(once)
	if once != twice {
		t.Fatalf("not idempotent:\n%q\n%q", once, twice)
	}
}

func TestBlankLinesPreserved(t *testing.T) {
	in := "function main() {\n\n    print(1)\n\n}\n"
	if got := Source(in); got != in {
		t.Fatalf("blank lines:\n%q", got)
	}
}
