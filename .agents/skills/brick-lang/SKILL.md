---
name: brick-lang
description: Work on the Brick compiler at /home/amritrai/brick — a small systems language that compiles to C and is being self-hosted. Use when editing .brick files, the Go toolchain, the self-hosted compiler, examples, tests or benchmarks; includes the invariants to preserve and a symptom-to-cause debugging playbook.
---

# Brick: how to work in this repo

Brick is a small, statically typed systems language. The compiler is a pipeline
of small stages, and every stage can be printed:

```
source .brick
  → token   (lexer)      brick tokens / brick tokdump
  → ast     (parser)     brick ast
  → checked (checker)    brick typedump
  → C       (codegen)    brick emit
  → executable           brick build / brick run   (gcc/clang does the rest)
```

There is also an experimental x86-64 backend (`--target native`, subset only)
and a tree-walking interpreter used for differential testing (`brick interp`).

**The Go compiler is the reference implementation. The self-hosted compiler in
`selfhost/` must produce byte-identical results.** That single rule generates
most of the work discipline below.

## Map

| Path | Role |
| --- | --- |
| `cmd/brick/main.go` | CLI, backend selection, `const version` (bump here) |
| `internal/codegen/codegen.go` | AST → C; contains the whole C runtime as Go string literals |
| `internal/checker/` | types, arity, diagnostics wording |
| `internal/native/` | experimental x86-64 backend |
| `internal/interp/` | interpreter; must agree with compiled programs |
| `selfhost/lexer.brick` … `codegen.brick`, `frontend.brick`, `brickc.brick` | the compiler rewritten in Brick |
| `selfhost/runtime.brick` | the C runtime, stored verbatim as string literals |
| `examples/*.brick` | runnable programs; they are the test corpus |
| `testdata/errors/**` | error-diagnostic corpus (diffed against `brick errdump`) |
| `bench/` | Brick vs C vs C++ vs Rust benchmark (`make bench`) |

## Commands

```sh
make build            # go build -o bin/brick ./cmd/brick  (do this after Go edits)
make test             # go test ./...
make vet; make fmt-check
make examples         # compile+run every example, showing generated C
make diff             # interpreter vs compiled binary, every example
make selfhost         # each Brick component vs the Go compiler's dump
make selfhost-e2e     # self-hosted compiler emits C, builds it, program matches
make bootstrap        # stage1 → stage2 → stage3, C must be identical
make bench            # four-language benchmark
make backup           # git bundle in ../brick-backups, verified
```

Single-file loop while developing:

```sh
./bin/brick emit prog.brick        # inspect the C
./bin/brick check prog.brick       # types only
./bin/brick errdump prog.brick     # first diagnostic, machine-comparable
./bin/brick diff prog.brick        # interpreter vs compiled
BRICK_CC=clang ./bin/brick build prog.brick -o prog
```

`./bin/brick` goes stale the moment you edit Go files — rebuild before believing
any `undefined function`-style error.

## Verification ladder

Run in this order; stop at the first failure and fix it there.

| Step | Catches |
| --- | --- |
| `go test ./...` | unit regressions in lexer/parser/checker/codegen/interp |
| `make fmt-check` | examples not canonically formatted |
| `make diff` | interpreter and compiled program disagree |
| `make selfhost` | a Brick component diverges from the Go one (named in the output) |
| `make selfhost-e2e` | the self-hosted compiler emits C that does not build or run |
| `make bootstrap` | stage2 and stage3 C differ (parity is broken somewhere) |
| `make bench` | checksum mismatch means a runtime change altered behaviour |

`selfhost-e2e` and `bootstrap` are slow (they compile the whole compiler several
times). For a small change, `go test`, `make selfhost` and one hand-built
program are usually enough; run the full ladder before committing.

## Hard invariants — check every one when you touch codegen

1. **Runtime helpers are gated.** A helper is emitted only if the program uses
   it. The Go side asks `programCalls(prog, "name")`; the Brick side asks
   `programCalls(em, name)`. A new builtin needs its own `usesX` flag in
   `emitProgram`, or every existing program's C changes (and the bootstrap
   fixpoint breaks).
2. **The runtime text is mirrored verbatim.** Anything you add or change in
   `internal/codegen/codegen.go` must appear byte-for-byte in
   `selfhost/runtime.brick`, which stores C inside escaped Brick strings
   (`\n`, `\"`, `\\`). Edit both, then let `make selfhost` prove it.
3. **Diagnostic wording is duplicated on purpose.** Every checker message in
   `internal/checker/checker.go` has an identical twin in
   `selfhost/frontend.brick`. `make selfhost` diffs them against
   `brick errdump` for every file in `testdata/errors/`.
4. **Watch the string delimiters.** The C runtime lives in a Go raw string
   (backticks): a backtick anywhere in a new comment ends it and the file stops
   compiling. In `selfhost/runtime.brick` the same text is a normal Brick
   string, so `"` and `\` need escaping.
5. **Emitted C must stay readable and warning-free**: K&R braces, `/* */`
   comments, `static inline`, named constants, `-Wall -Wextra` clean.
6. **Every identifier that comes from a Brick source goes through `cname`.**
   `cReservedNames` (`internal/codegen/codegen.go`) and `cReservedNames`
   (`selfhost/codegen.brick`) are the *same list, word for word*: C keywords,
   the five always-included headers, and the `brick_*` runtime. A reserved
   name is written as `bx_<name>` in the C output. If you add an emitter site
   that writes a name (a new declaration kind, a new expression form), it must
   call `cname` too, or a program named after a C word emits C that will not
   compile. `extern` function names are the exception: they name real C
   symbols, so they are never renamed. The Go list is a raw string with
   newlines; the Brick one must be a **single line** because the Brick emitter
   does not fold concatenated string constants.

## Adding a language feature or builtin — the full checklist

1. `internal/checker/checker.go` — infer the call's type, check arity and
   argument types, with an error message that names the builtin and the types.
2. `internal/codegen/codegen.go` — add the `CallExpr` case, the runtime helper
   (as a Go string), and `usesX` + emission gate in `emitProgram`.
3. `selfhost/frontend.brick` — the same type rules and the same message text.
4. `selfhost/runtime.brick` — the same C bytes.
5. `selfhost/codegen.brick` — the same emission case, gate and expression form.
6. `internal/interp/interp.go` — the same semantics (used by `brick diff`).
7. `internal/native/native.go` — add the name to the unsupported list so
   `--target native` fails cleanly and `--target auto` falls back to C.
8. Tests: `internal/checker/checker_test.go` (accept + reject),
   `internal/codegen/codegen_test.go` (emitted C, presence and absence),
   `internal/interp/interp_test.go` (behaviour).
9. `testdata/errors/check/*.brick` for each new message; verify with
   `make selfhost`.
10. `examples/*.brick` that actually runs, passing `make diff` and
    `make fmt-check`.
11. README: "What works today" bullet + version-table row; `selfhost/README.md`
    if the self-hosting story changed.
12. Bump `const version` in `cmd/brick/main.go`, commit, `git tag -a vX.Y.Z`,
    `make backup`.

## Debugging playbook: symptom → cause → rule

Before diagnosing anything, rebuild: `make build`.

| Symptom | Most likely cause | What to do |
| --- | --- | --- |
| `undefined function "x"` for a builtin that exists | stale `./bin/brick` | `make build` |
| `duplicate function "x"` in a `selfhost/` file | the helper is already defined in a shared file (`frontend.brick` is included by `brickc.brick`) | delete the new copy, call the shared one |
| `"i" is already declared in this scope` | Brick forbids redeclaring a name in the same scope | rename the second variable |
| `missing ',' in argument list` in `codegen.go` | a backtick inside the C runtime raw string | rewrite the comment without backticks |
| `make selfhost` reports a mismatch for one component | the two implementations diverged | fix the Brick side (or the Go side if the Brick one is right), then re-run; the target names the component |
| `bootstrap: stage 2/3 C differ` | a runtime helper is emitted unconditionally, or text differs by a byte | diff the two generated C files, look for an ungated helper or escaping |
| `multiple definition of 'main'` when building `foo.brick` | `foo.c` or `foo.d/*.c` next to the source was auto-linked as a companion | rename the C file (`bench-c.c`, not `bench.c`) |
| link error `undefined reference to brick_x` | the helper is used but gated off by `usesX` | add the name to the gate's `programCalls` check on both sides |
| `brick: index out of bounds: N` from the self-hosted compiler | an argument list was indexed before an arity check | check `args.len` first, then index |
| self-hosted output differs only in whitespace/indentation | the re-indenter state machine (`\t` counter) lost a level | compare with `brick emit` and re-check the block that changed |
| interpreter and compiled program disagree | `internal/interp` and `internal/codegen` implement different semantics | `brick diff` the example, decide which matches the spec, fix the other |
| error message text differs from `brick errdump` | wording duplicated in `frontend.brick` drifted | copy the Go message verbatim |
| examples fail `fmt-check` | hand-written layout | `./bin/brick fmt examples/foo.brick` |
| bootstrap or a build hangs | an infinite loop introduced by a bulk text replace (`s.replace("eprint(", ...)` rewriting `report` itself) | inspect the function you just touched; re-edit explicitly |
| checker panics instead of returning an error | intentional: the Go checker reports the first error by panicking, `cmd/brick` catches it | do not "fix" the panic; fix the caller's message |

Golden rule when two implementations disagree: **the Go compiler is the
reference**. Change the Brick side to match it, unless you have a spec reason to
change both — in which case change both, and let `make selfhost` and
`make bootstrap` prove it.

## Writing Brick code (gotchas)

- `List(T)` grows with `.push`; index with `xs[i]`, length with `.len`.
- `Builder()` accumulates with `.add(text)` / `.build()`; `builder.add(to_text(n))`
  takes an integer fast path.
- No redeclaration in the same scope, no hex literals, no triple-quoted strings,
  no `List(int){...}` literal. `extern` is a keyword.
- `print` takes text/int/bool/float/bytes, not a reference.
- `text` concatenation with `+`; `to_text`/`to_bytes`/`bytes` are the explicit
  conversions (there are no implicit ones).
- The interpreter is a second implementation: anything you rely on must behave
  the same there (`brick diff`), or examples will fail.
- Names C owns (`double`, `long`, `printf`, `strlen`, `size_t`, `case`, …) are
  legal Brick names; the emitter writes `bx_double` and friends. Type names
  (`int`, `float`, `bool`, `text`, `bytes`, `void`) and builtins (`print`,
  `exit`, `slice`, …) are not: those are errors. `examples/reserved.brick`.
- **`text.len` and `slice(text, ...)` are `strlen`.** A `text` is a plain
  `const char *`, so length is recomputed every time it is asked for. Never read
  `.len` of a big text inside a per-character/per-token loop and never slice a
  big text per token — hoist the length into a field (`Lexer.srcLen`) and build
  literals from bytes (`Builder` + a one-byte `text`). Reading `src.len` per
  character is what made the self-hosted compiler quadratic (15 s → 0.2 s on a
  6,400-function file).

## Performance work

`make bench` builds the same three workloads in Brick, C, C++ and Rust, refuses
to time a workload whose checksum disagrees, pins to one core, interleaves the
four programs per round and reports the best round. `RUNS=n` and `PIN=<core>`
adjust it. This machine is shared; single timings are meaningless, ratios are
stable. Optimisations belong in the runtime (see the `Builder` integer fast
path) rather than in exotic codegen, and must not change emitted C for programs
that do not use the feature.

Compiler throughput is a second dimension: time `./bin/brick emit FILE` (front
end only) against the C build, and time the self-hosted `/tmp/brickc FILE -o -`
on synthetic inputs of 400/1,600/6,400 generated functions to catch superlinear
blowups. Emitted-C compiling dominates `brick build` (cc -O2 on a 4.5 kLOC file
is ~1.4 s of a ~1.7 s build), so only the front end is worth optimizing.

## Versioning and rollback

Each user-visible change gets: tests green → README row → `const version` bump →
`git commit` → `git tag -a vX.Y.Z` → `make backup`. Backups are verified git
bundles in `../brick-backups/`; restore with
`git clone ../brick-backups/brick-YYYYMMDD-HHMM.bundle brick`.
