# Self-hosting Brick

The long-term goal (spec §15, roadmap item 9) is to rewrite the Brick compiler
in Brick itself. This directory holds that effort, one component at a time.
Each component is verified against the Go compiler so the two can never drift.

## How a component is checked

The Go toolchain exposes a canonical, escape-free token dump:

```sh
brick tokdump examples/hello.brick
```

Each line is `line col KIND nbytes b0 b1 ...` — the literal written as decimal
byte values so no quoting or escaping rules have to agree between Go and Brick.

A self-hosted component reads a `.brick` file, produces exactly that dump, and
`make selfhost` diffs the two over every example:

```sh
make selfhost        # build the Brick lexer, run both, diff each example
```

When the dumps match for every example, the Brick lexer is a drop-in
replacement for `internal/lexer`. It currently matches on **every**
`examples/*.brick` file (`make selfhost`).

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

| Component | Brick file | Verified against |
| --- | --- | --- |
| Lexer | `lexer.brick` | `brick tokdump` — matches on all examples |
| Parser | — | `brick ast` (planned) |
| Checker | — | planned |
| C emitter | — | `brick emit` (planned) |

## Building the lexer by hand

```sh
brick build selfhost/lexer.brick -o /tmp/brick-lex
/tmp/brick-lex examples/hello.brick
```
