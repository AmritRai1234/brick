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

`brick tokdump` prints a canonical, escape-free token dump — each line is
`line col KIND nbytes b0 b1 ...`, the literal written as decimal byte values so
no quoting or escaping rules have to agree between Go and Brick. `brick ast`
prints the tree as indented text.

```sh
make selfhost        # build both components, run both, diff every example
```

Both currently match on **every** `examples/*.brick` file.

## Layout

- `tokens.brick` — the scanner, shared by both components.
- `lexer.brick` — the token-dump front end (`import "tokens.brick"`).
- `parser.brick` — the parser + AST dumper (`import "tokens.brick"`); it also
  resolves imports exactly like `internal/loader`.

The parser renders the dump text directly during recursive descent instead of
building an AST: each parse function returns its node's dump text indented
relative to itself, and the caller shifts the whole block right one level. New-
and tab-bytes inside string literals are temporarily escaped so the re-indenter
only sees structural line breaks (the Go dump prints them raw).

## Known gap: embedded NUL bytes

Brick's `text` is a NUL-terminated C string, so it cannot carry an embedded
`\0`. A string or character literal whose value contains a NUL therefore
truncates in the Brick lexer (e.g. `"a\0b"` yields `a b` instead of `a NUL b`),
while the Go lexer keeps all three bytes. Brick programs cannot use such a
literal at runtime anyway (the same truncation happens in generated code), so
this only shows up in the token dump.

A byte-exact lexer needs a length-carrying byte-string type in the language
(`bytes`/`buffer`). That is the first prerequisite for finishing this port
faithfully and is tracked as the next self-hosting step.

## Status

| Component | Status |
| --- | --- |
| Lexer | done — matches `brick tokdump` on all examples |
| Parser (+ AST dump) | done — matches `brick ast` on all examples |
| Checker | planned |
| C emitter | planned (`brick emit`) |

## Building a component by hand

```sh
brick build selfhost/parser.brick -o /tmp/brick-parse
/tmp/brick-parse examples/hello.brick
```
