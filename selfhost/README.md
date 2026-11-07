# Self-hosting Brick

The long-term goal (spec §15, roadmap item 9) is to rewrite the Brick compiler
in Brick itself. This directory holds that effort, one component at a time.
Each component is verified against the Go compiler so the two can never drift.

## How a component is checked

Each component is diffed against the matching Go compiler command over every
example, so the two can never drift:

| Component | Brick file | Go command | Brick command |
| --- | --- | --- | --- |
| Lexer | `lexer.brick` | `brick tokdump` | `brick run selfhost/lexer.brick` |
| Parser | `parser.brick` | `brick ast` | `brick run selfhost/parser.brick` |
| Checker | `checker.brick` | `brick typedump` | `brick run selfhost/checker.brick` |
| C emitter | `emitter.brick` | `brick emit` | `brick run selfhost/emitter.brick` |
| Whole compiler | `brickc.brick` | `brick emit` | `brick run selfhost/brickc.brick` |

`brick tokdump` prints a canonical, escape-free token dump — each line is
`line col KIND nbytes b0 b1 ...`, the literal written as decimal byte values so
no quoting or escaping rules have to agree between Go and Brick. `brick ast`
prints the tree as indented text. `brick typedump` prints the inferred type of
every expression, one `line:col KIND type` line each, in source order — a
fingerprint of the checker's work with no formatting left to agree on.
`brick emit` prints the generated C, so the C emitter is checked by diffing the
whole program text.

```sh
make selfhost        # build every component, run it, diff every example
make selfhost-e2e    # compile examples with the single Brick compiler, run them, compare
```

Every component currently matches on **every** `examples/*.brick` file, and on
the imported library `examples/lib/greet.brick`. `make selfhost-e2e` goes
further: it uses the single self-hosted compiler (`brickc.brick`) as the
compiler, builds its C with the system C compiler, runs the binary, and checks
the program behaves the same as one built by the Go compiler.

## The single compiler

`brickc.brick` is the whole pipeline as one program — lex, parse, resolve
imports, check, emit — the same stages the separate components verify, wired
into one command. Brick cannot spawn a process yet, so `brickc` stops at C: it
writes the generated C to a file (default: the input name with a `.c`
extension).

```sh
brick run selfhost/brickc.brick examples/hello.brick           # writes examples/hello.c
brick run selfhost/brickc.brick examples/hello.brick -o out.c  # writes out.c
brick run selfhost/brickc.brick examples/hello.brick -o -      # prints to stdout
```

The two drivers share the emit machinery in `codegen.brick`: `emitter.brick`
prints to stdout (for the `brick emit` diff), `brickc.brick` writes a file.

## Layout

- `tokens.brick` — the scanner, shared by every component.
- `frontend.brick` — the shared front end: tokenize, parse into a flat node
  arena, resolve imports like `internal/loader`, and run the checker's semantic
  analysis (so every expression node carries its inferred type). Imported by the
  self-hosted checker and C emitter, which add their own output pass and `main`.
- `lexer.brick` — the token-dump front end (`import "tokens.brick"`).
- `parser.brick` — the parser + AST dumper (`import "tokens.brick"`); it also
  resolves imports exactly like `internal/loader`.
- `checker.brick` — the checker + inferred-type dumper (`import "frontend.brick"`);
  it prints the `brick typedump` fingerprint.
- `codegen.brick` — the C backend: the emitter struct and every emit function,
  plus `renderProgram`, which returns the generated C as text. Imported by the
  two drivers (`emitter.brick`, `brickc.brick`).
- `emitter.brick` — a driver over `codegen.brick` that prints the C to stdout,
  matching `brick emit`.
- `brickc.brick` — the single self-hosted compiler: a driver over
  `codegen.brick` that writes the C to a file.
- `runtime.brick` — the C runtime prelude, stored verbatim (as string literals)
  so the emitter emits the exact same bytes the Go compiler does.

The parser renders the dump text directly during recursive descent instead of
building an AST: each parse function returns its node's dump text indented
relative to itself, and the caller shifts the whole block right one level. New-
and tab-bytes inside string literals are temporarily escaped so the re-indenter
only sees structural line breaks (the Go dump prints them raw).

The checker cannot reuse that trick because it needs the tree twice (once to
collect declarations, once to check bodies and infer types). Brick has no
pointer or recursive types, so — following the spec's "indices instead of
pointers for linked data" — it builds a **flat arena**: every node lives in a
`List(Node)` and children are referenced by `int` index. Types are likewise kept
as canonical text (`"int"`, `"List(int)"`, `"&Point"`, `"Map(text, int)"`), so
type equality is a string compare.

The C emitter reuses that arena: after `collect` and the body check have filled
in every node's type, it walks the tree in the same order as `internal/codegen`
and writes the same lines with the same indentation. The runtime prelude is not
retyped — it is extracted verbatim into `runtime.brick` — so only the logic has
to agree, not 400 lines of C.

## Known gap: embedded NUL bytes

Brick's `text` is a NUL-terminated C string, so it cannot carry an embedded
`\0`. A string or character literal whose value contains a NUL therefore
truncates in the Brick lexer (e.g. `"a\0b"` yields `a b` instead of `a NUL b`),
while the Go lexer keeps all three bytes. Brick programs cannot use such a
literal at runtime as `text` (the same truncation happens in generated code),
so this only shows up in the token dump.

The language now has a length-carrying byte-string type, `bytes`, which can
hold NUL and any byte 0..255; the self-hosted front end and code generator
understand it, and `examples/bytes.brick` exercises it end to end. Switching
the self-hosted lexer's token literal and source buffer from `text` to `bytes`
is what remains to make the port byte-exact on sources containing NUL.

## Status

| Component | Status |
| --- | --- |
| Lexer | done — matches `brick tokdump` on all examples (text sources; NUL gap above) |
| Parser (+ AST dump) | done — matches `brick ast` on all examples |
| Checker (+ type dump) | done — matches `brick typedump` on all examples |
| C emitter | done — matches `brick emit` on all examples, including `bytes` |
| Single compiler (`brickc`) | done — source in, C out; C compiles and runs like the Go compiler's |

## Building a component by hand

```sh
brick build selfhost/parser.brick -o /tmp/brick-parse
/tmp/brick-parse examples/hello.brick
```
