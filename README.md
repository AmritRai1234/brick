# Brick

A small, fast, readable systems language — *bare metal, memory-safe, no garbage
collector, no lifetime annotations*. Brick compiles to C and lets gcc/clang do
the optimizing.

This repository is the **first compiler**, written in Go, following the
pipeline in the design notes: `text → tokens → tree → checks → C`.

> The full language design lives in [`docs/SPEC.md`](docs/SPEC.md). This README
> describes what the compiler *currently does*.

## Quick start

```sh
make build              # builds ./bin/brick
./bin/brick run examples/hello.brick
./bin/brick run examples/math.brick      # -> 55
./bin/brick emit examples/point.brick    # show the generated C
```

You need **Go 1.24+** and a C compiler (`cc`, `gcc`, or `clang`). Override the C
compiler with `BRICK_CC=clang`.

**New to programming?** Start with [`docs/learn/index.html`](docs/learn/index.html)
— a from-zero, twelve-lesson guide in plain English. Open it in any browser.

## Commands

| Command | What it does |
| --- | --- |
| `brick run <file> [args...]` | Compile, link, and run a program |
| `brick build <file> [-o out]` | Compile to a native executable |
| `brick emit <file>` | Print the generated C (nothing hidden) |
| `brick asm <file>` | Print the native x86-64 assembly |
| `brick check <file>` | Type-check only |
| `brick fmt <file>` | Reformat in place (`--stdout` prints, `--check` verifies) |
| `brick lsp` | Run the language server on stdio |
| `brick interp <file> [args...]` | Run on the tree-walking interpreter |
| `brick diff <file> [args...]` | Compile+run and interpret, compare output |
| `brick tokens <file>` | Dump the token stream |
| `brick ast <file>` | Dump the parsed tree |
| `brick version` | Print the compiler version |

`make bench` runs the same three workloads written in Brick, C, C++ and Rust and
reports their timings side by side; see `bench/README.md` for the numbers and
what they show. `make bench-server` does the same for a very small HTTP server
(Brick, C, Go, Rust) under ApacheBench; see `bench/server/README.md`.

Every stage can be inspected — a deliberate design goal ("stage dump flag.
Print tokens, tree, core form and output so nothing is hidden").

## Two backends

Brick can lower to C **or** straight to x86-64 machine code. Pick with
`--target`:

| `--target` | What happens | Best for |
| --- | --- | --- |
| `c` **(default)** | Emit C; gcc/clang optimize and assemble | Everything — this is the supported path |
| `native` | Emit x86-64 assembly directly (System V ABI); `as` encodes, `cc` only links libc | Experiments; no C in the codegen path |
| `auto` | Try `native`, fall back to `c` for unsupported features | When you want to see the native path |

```sh
brick run examples/math.brick                    # uses the C backend
brick run --target native examples/math.brick    # experimental native path
brick asm   examples/math.brick                  # see the x86-64 we generate
```

### Why both?

**The C backend is the default and the supported path.** Compiling to C is not
a compromise: gcc/clang are world-class optimizers, and the Brick compiler stays
small by not re-implementing instruction selection, register allocation and
optimization. Every feature is guaranteed to work through C.

A native x86-64 backend exists as an **experiment** (`--target native`). It
proves Brick can own its code generation, but it has no optimizer yet, so on a
100M-iteration loop its output is ~30–50× slower than the same program through
gcc -O2 — speed comes from the optimizer, not from which text you emit. It is
kept, opt-in, as the place where a future optimizer (or an LLVM IR backend)
would live.

## Example

```brick
enum Result { Ok(int), Err(text) }

function divide(a: int, b: int) -> Result {
    if b == 0 {
        return Result.Err("division by zero")
    }
    return Result.Ok(a / b)
}

function main() {
    match divide(10, 2) {
        Result.Ok(n) => print(n)
        Result.Err(msg) => print(msg)
    }
}
```

Errors are values, not exceptions — `match` forces you to handle every variant.

Collections and text:

```brick
function main() {
    var xs = List(int)
    xs.push(10)
    xs.push(20)
    var i = 0
    while i < xs.len {
        print(xs[i])
        i = i + 1
    }
    print("hi" + " there")   // hi there
}
```

```brick
struct Point { x: int, y: int }

function move(p: &mut Point) {
    p.x = p.x + 1
    p.y = p.y + 1
}

function main() {
    var a = Point{ x: 1, y: 2 }
    move(&mut a)
    print(a.x)   // 2
    print(a.y)   // 3
}
```

Maps are keyed by `text`, with `set`, `get` (panics on a missing key, like an
out-of-range list index) and `has`:

```brick
function main() {
    var counts = Map(text, int)
    counts.set("apples", 3)
    counts.set("pears", 1)
    counts.set("apples", 5)      // overwrite
    print(counts.len)            // 2
    print(counts.get("apples"))  // 5
    print(counts.has("plums"))   // false
}
```

Arenas free a whole region at once, which is often faster than `malloc`/`free`
per object:

```brick
function main() {
    arena {
        var xs = List(int)
        var i = 0
        while i < 1000 {
            xs.push(i)
            i = i + 1
        }
        print(xs.len)
    }   // every allocation above is released here, in one pass
}
```

A program can span several files. `import` pulls another file's top-level
declarations into the program (there is no separate compilation — the compiler
sees one unit):

```brick
// main.brick
import "lib/greet"

function main() {
    print(greet("brick"))   // defined in lib/greet.brick
}
```

`Builder` assembles text without quadratic `+`, and `to_text` turns a number
into text (Brick has no implicit conversions):

```brick
function main() {
    var out = Builder()
    var i = 1
    while i <= 3 {
        out.add("item ")
        out.add(to_text(i))
        out.add("\n")
        i = i + 1
    }
    print(out.build())
}
```

Text is a NUL-terminated C string, so it cannot hold a zero byte. A `bytes`
value carries its own length and can hold any byte 0..255 — the prerequisite
for a lexer over arbitrary source files:

```brick
function main() {
    var codes = List(int)
    codes.push(104)   // h
    codes.push(105)   // i
    codes.push(0)     // NUL is kept, because bytes carries a length
    var b = bytes(codes)
    print(b.len)      // 3
    print(b[2])       // 0
    print(to_text(to_bytes("hi")))   // hi
}
```

`to_bytes(text)` copies a string's bytes up to the first NUL; `bytes(List(int))`
builds one from byte values; `to_text(bytes)` copies it back. Indexing and
`for b in ...` yield `int` bytes, and `==` / `!=` compare them byte-wise.
`print_bytes(b)` writes the buffer exactly — NULs included, no newline added —
which is how the self-hosted dumps emit byte-exact output.

Generates readable C you can read side-by-side with the source (K&R style,
Clean Code naming, `static` linkage, no hidden state):

```c
typedef struct Point Point;

struct Point {
    int64_t x;
    int64_t y;
};

static void move(Point* p)
{
    (p)->x = ((p)->x + 1);
    (p)->y = ((p)->y + 1);
}

int main(void)
{
    Point a = (Point){ .x = 1, .y = 2 };
    move((&(a)));
    printf("%lld\n", (long long)((a).x));
    printf("%lld\n", (long long)((a).y));
    return 0;
}
```

## What works today

Mapped to the roadmap in the spec (§15):

- [x] **1. Hello world** — reads a `.brick` file, writes equivalent C
- [x] **2. Variables and math** — `var x = 2 + 3`, `print(x)`; `int`, `float`,
      `bool`, `text`; arithmetic, comparison and logical operators; top-level
      `const NAME = <compile-time expression>` (folded and inlined)
- [x] **3. `if` / `else` and `while`**, plus `break` and `continue` (which
      leave / advance the innermost loop, even from inside a `match`), and
      `for x in xs { ... }` — iterate a `List(T)` (binding each element) or
      `text` (binding each byte as an `int`). `for` is pure sugar over `while`
      (§5), so a `continue` still advances to the next item.
- [x] **4. `function`** — typed parameters and return types, recursion,
      forward references; builtin `print`
- [x] **5. `struct`, then `extern`** — struct literals, field access, borrowing
      with `&` / `&mut` (temporary only), and calls into C
- [x] **6. Borrowing, arenas, `enum`/`match`, errors** — `enum` + `match`:
      tagged variants with payloads, bindings, wildcard, and exhaustiveness
      checking. Errors-as-values (§7) work via enums. Borrowing with `&`/`&mut`.
      `arena { ... }` opens a scoped region (§6): allocations inside come from a
      chunked bump allocator and are freed together at the block's closing
      brace. Arena-local names do not escape the block (see `examples/arena.brick`).
- [x] **Collections and text** — `List(T)` (growable: `push`, indexing, `.len`,
      assign to an index) and text (`.len`, `s[i]` as a byte, `+` concatenation,
      `==` / `!=`), plus character literals (`'a'` = 97) so byte-wise scanning
      reads clearly. These are the pieces a lexer and parser are built from.
- [x] **7. Formatter and language server** — the formatter (`brick fmt`,
      `make fmt-check`) normalizes indentation (four spaces per level) and strips
      trailing whitespace while preserving every comment. `brick lsp` runs a
      small language server over stdio (JSON-RPC, Content-Length framing): it
      publishes diagnostics from the parser and checker and answers formatting
      requests, reusing the same front end as the compiler.
- [x] **8. Interpreter for differential testing** — a slow tree-walking
      interpreter (`internal/interp`) runs the same programs as the compiled
      binary; `brick diff` and `make diff` compare their output and report any
      disagreement (§10). It covers the whole core language.
- [~] **9. Self-hosting** — in progress. The lexer (`selfhost/lexer.brick`),
      the parser + AST dumper (`selfhost/parser.brick`), the checker +
      type dumper (`selfhost/checker.brick`) and the C emitter
      (`selfhost/emitter.brick`) are written in Brick and checked against the
      Go compiler (`brick tokdump`, `brick ast`, `brick typedump`, `brick emit`)
      by `make selfhost` over every example. `selfhost/brickc.brick` is the
      whole pipeline as one program — lex, parse, resolve imports, check, emit —
      a single self-hosted compiler; `make selfhost-e2e` uses it to compile each
      example, builds the C with the system C compiler, and checks the result
      runs like the Go compiler's. The length-carrying `bytes` type plus
      `print_bytes` made the whole front end byte-exact: token literals, AST
      string nodes and the dumps carry exact bytes, so every stage matches the
      Go compiler on `selfhost/*.brick` too, whose sources contain `\0`
      literals (see `selfhost/README.md`). It also builds itself:
      `make bootstrap` builds stage 1 with the Go compiler, uses stage 1 to
      compile `brickc.brick` into stage 2, and stage 2 into stage 3 — every
      stage emits identical C for every input, and stage 2 and stage 3 are
      byte-identical, so the compiler has reached a fixpoint. Its diagnostics
      match too: like the Go front end it stops at the first error, words it
      the same way, and (via the new `exit(status)` builtin) exits non-zero
      without writing C. `brick errdump` prints that first diagnostic in a
      canonical form, and `make selfhost` diffs it against the self-hosted
      parser, checker, emitter and `brickc` over `testdata/errors/`. The last
      gap was the driver: a compiler that cannot start another program cannot
      link what it emits, so `system(argv)` now runs a program with exact
      arguments and returns its exit status — the piece the self-hosted
      compiler needs to hand its C to a C compiler. It does: `brickc prog.brick
      -o prog` writes the C and then runs `$BRICK_CC` (or `cc`) on it, so one
      self-hosted command turns Brick source into a working executable, and
      `make bootstrap` checks that the compiler it just built can build itself
      again *and* link a program that runs. With `read_dir` / `is_dir` it also
      resolves `extern` companion C files (`foo.c`, `foo.d/*.c`) exactly like
      the Go driver, so `examples/extern.brick` is built by the self-hosted
      compiler too — for programs in this repository, no Go code is needed
      anywhere in the chain from source to executable.

### Semantics enforced by the checker

- No implicit conversions: `var x: float = 3` is an error, as is `1 + "two"`.
- `if`/`while` conditions must be `bool`; `&&`/`||`/`!` are bool-only.
- Functions and struct fields must be fully and correctly specified.
- Borrows are types (`&Point`), so a `&mut Point` cannot be passed where an
  `int` is expected, and vice versa.
- Only declarations are allowed at the top level; code lives in a function.
- Names that C already owns are yours to use. Brick emits C, and that C always
  includes `<stdint.h>`, `<stdbool.h>`, `<stdio.h>`, `<stdlib.h>` and
  `<string.h>`, so names such as `double`, `long`, `printf`, `strlen`, `size_t`
  or `struct` would normally clash. They do not: the emitter writes `bx_` in
  front of them (`double` becomes `bx_double`), so a program may name a
  function `double` and a field `printf`. Two names remain unavailable because
  the language itself uses them — the type names (`int`, `float`, `bool`,
  `text`, `bytes`, `void`) and the built-in functions (`print`, `exit`,
  `slice`, …); both produce a clear diagnostic instead of broken C.
  `examples/reserved.brick` is the demonstration.

### Backend coverage

The **C backend** handles the full language described above. The **native
x86-64 backend** currently covers `int`, `bool`, `text`, arithmetic, all
comparisons, `&&`/`||`/`!`, functions (recursion, up to 6 args), `if`/`while`,
`var`/assignment, and `print`. Floats, structs, and `&`/`&mut` references return
an `UnsupportedError` and trigger the `auto` fallback to C.

## Project layout

```
cmd/brick/          CLI entry point
internal/token/     token kinds and keywords
internal/lexer/     text -> tokens
internal/parser/    tokens -> AST (hand-written recursive descent)
internal/ast/       AST nodes and debug printer
internal/checker/   symbol tables, type checks, expression types
internal/codegen/   AST -> C
internal/native/     AST -> x86-64 assembly (Brick's own backend)
internal/loader/     source + imports -> one merged AST
internal/interp/     AST -> values (tree-walking interpreter, for diff testing)
internal/format/     source -> canonically indented source (the formatter)
internal/lsp/        language server (JSON-RPC over stdio)
docs/SPEC.md        the language design notes
examples/           runnable Brick programs
bench/              the same three workloads in Brick, C, C++ and Rust (make bench)
bench/server/       a very small HTTP server in Brick, C, Go and Rust (make bench-server)
AGENTS.md           working notes for AI agents: rules, commands, debugging playbook
.agents/skills/brick-lang/  the same as a loadable skill, plus verify.sh (the gate)
```

## Testing

```sh
make test        # go test ./...
make examples    # build every example and run it
```

The lexer, parser, checker and code generator each have unit tests, and every
example in `examples/` is a working end-to-end case.

## Getting to self-hosting

Step 9 in the roadmap (§9, §15) is to rewrite the compiler in Brick. That needs
the language to be able to express a compiler: symbol tables, source text in and
C text out, and enough control flow. This is the working audit of what a
Brick-written compiler would need.

**Ready today**

- structs, `List(T)`, `Map(text, V)`, `text`, `int`, `float`, `bool`
- `enum` + `match` (the natural way to represent tokens and AST nodes)
- `while` and `for ... in` with `break` / `continue`
- borrowing (`&`, `&mut`) — e.g. `&mut xs[0]` passed into a function
- structs holding lists, and `xs[i].field = v`
- character literals (`'a'`, `'\n'`) — a byte literal that is just its `int`
  value, so it compares and indexes naturally with `text`
- reading and writing files (`read_file`, `write_file`), command-line `args()`
- `import "path"` — pull another file's declarations into the program
- `Builder()` (`add`, `build`) and `to_text(...)` for `int`, `float`, `bool`
  and `bytes` — assemble output without quadratic `+`
- `bytes` — a length-carrying byte string that can hold NUL and any byte
  0..255, built with `bytes(List(int))` or `to_bytes(text)`, read back with
  `to_text`, `.len`, `b[i]`, `for b in ...` and `==` / `!=`, and written
  exactly with `print_bytes`
- top-level `const` — computed at compile time and inlined
- `arena { ... }` for bulk allocation
- `slice(text, start, end) -> text` — extract a byte range of a string
- output via `print` / `eprint`, and `exit(status)` to stop a program with a
  status code (the interpreter and the compiled binary both honour it)
- `system(argv)` — run another program with `List(text)` arguments, no shell in
  between, and get its exit status (`127` when it cannot be started); this is
  what lets a Brick program drive a C compiler
- `getenv(name)` — an environment variable, `""` when it is not set
- `read_dir(path)` — the names in a directory, sorted and without `.` / `..`
  (empty when it cannot be opened), and `is_dir(path)` — whether a path exists
  and is a directory

With those in place, every box in the audit is ticked: Brick can express a
lexer, a parser, symbol tables, type checking and a C emitter. The roadmap's
last item (self-hosting, §15) is now a matter of writing the compiler in Brick
itself — and that has happened: the lexer (`selfhost/lexer.brick`), parser
(`selfhost/parser.brick`), checker (`selfhost/checker.brick`) and C emitter
(`selfhost/emitter.brick`) are self-hosted and verified against `brick tokdump`,
`brick ast`, `brick typedump` and `brick emit` by `make selfhost`, and
`selfhost/brickc.brick` wires all four stages into one self-hosted compiler
(source in, C out) that `make selfhost-e2e` exercises end to end and that
`make bootstrap` shows to compile itself.

Each feature above landed the same way as the ones before it: implemented across
the lexer/parser/checker/codegen/interpreter, with an example that `make diff`
proves the interpreter and the compiled binary agree on.

## Version control

The repo is tagged at every milestone, so an older version is always one command
away:

| Tag | Milestone |
| --- | --- |
| `v0.1.0` | first compiler (lexer, parser, checker, C + native backends, enums, lists, text) |
| `v0.2.0` | I/O and process builtins (`eprint`, `read_file`, `write_file`, `args`) |
| `v0.2.1` | interpreter + differential testing, `extern` linked end to end |
| `v0.2.2` | the official formatter |
| `v0.2.3` | arenas |
| `v0.3.0` | language server |
| `v0.3.1` | `break` / `continue` + self-hosting audit |
| `v0.3.2` | `Map(text, V)` |
| `v0.3.3` | `import` / multi-file programs |
| `v0.3.4` | text `Builder` + `to_text(int)` |
| `v0.3.5` | top-level `const` |
| `v0.3.6` | `for x in xs { ... }` |
| `v0.3.7` | character literals (`'a'`) |
| `v0.3.8` | `to_text` for `float` and `bool` |
| `v0.3.9` | predictable C linking (`foo.c`, `foo.d/*.c`) |
| `v0.3.10` | self-hosting begins: `slice`, `brick tokdump`, Brick-written lexer |
| `v0.3.11` | self-hosted parser + AST dump; borrow keeps `List` element type |
| `v0.3.12` | `brick typedump`: canonical inferred-type dump for self-host checks |
| `v0.3.13` | self-hosted checker + type dump |
| `v0.3.14` | self-hosted C emitter; shared `frontend.brick` |
| `v0.3.15` | single self-hosted compiler (`brickc.brick`); shared `codegen.brick` |
| `v0.3.16` | `bytes`: length-carrying byte strings |
| `v0.3.17` | byte-exact self-hosted lexer: token literals as `bytes` |
| `v0.3.18` | `print_bytes`; byte-exact AST dump and emitter on `\0` literals |
| `v0.3.19` | bootstrap: the self-hosted compiler compiles itself (identical C, stage 2 = stage 3) |
| `v0.3.20` | `exit(status)`; `brick errdump`; self-hosted diagnostics match the Go compiler |
| `v0.3.21` | `system(argv)`: run a program without a shell and read its exit status |
| `v0.3.22` | `getenv`; the self-hosted compiler builds and links programs by itself |
| `v0.3.23` | `read_dir` / `is_dir`; `brickc` links `extern` companion C files |
| `v0.3.24` | `builder.add(to_text(n))` appends digits straight into the buffer; `to_text` for ints no longer uses `snprintf` |
| `v0.3.25` | `AGENTS.md` + the `brick-lang` agent skill: invariants, verification ladder, debugging playbook |
| `v0.3.26` | self-hosted compiler is linear again: cache `text.len` in the scanner, stop slicing the source per token (~70x on large inputs) |
| `v0.3.27` | four-way HTTP server benchmark (`make bench-server`): Brick matches hand-written C at ~20–26 µs of CPU per request; Go ~1.6–2.2x, Rust ~1.1–1.3x |
| `v0.3.28` | `docs/learn/index.html`: a from-zero, twelve-lesson guide to Brick for people who have never written code |
| `v0.3.29` | C's names are no longer a trap: a Brick program may declare `double`, `printf`, `strlen`, `size_t`, `long`, … and the emitter writes them as `bx_…` in the C it produces (`examples/reserved.brick`) (current) |

```sh
git tag                     # list the tags
git checkout v0.2.3         # look at an older version (detached HEAD)
git checkout master         # come back to the latest

git restore .               # throw away uncommitted edits
git clean -fd               # also remove new untracked files
git checkout .              # (older spelling of `git restore .`)
```

A full single-file backup of the whole repo — every branch and tag — can be
written to `../brick-backups` and restored anywhere:

```sh
make backup                              # writes ../brick-backups/brick-<date>.bundle
git clone brick-<date>.bundle brick      # restore it as a fresh repo
```

## Design notes

- **One obvious output shape per construct.** Each core construct lowers to a
  fixed C template, so source and output read in parallel.
- **The emitted C follows K&R + Clean Code conventions** — K&R brace style,
  `/* */` comments, section banners, intention-revealing names, named constants,
  `static` linkage for internal functions, and `const` where possible — so the
  intermediate C has no landmines when we later generate it from Brick itself.
  It compiles clean under `-Wall -Wextra`.
- **Recursive descent, not a generator.** Small and regular syntax means a
  hand-written parser gives better errors and is easier to debug.
- **C as the backend.** All the interesting optimization happens in gcc/clang;
  the Brick compiler stays small enough to reason about and, later, to verify.
  The one place the *runtime* beats a generic library is appending integers to a
  `Builder`: `builder.add(to_text(n))` compiles to `brick_builder_add_int`, which
  writes the digits into the buffer directly, so building text allocates nothing
  per number (see `bench/`).
- **`text.len` is a `strlen`, and so is `slice`.** A `text` is a plain
  `const char *`, so its length is recomputed on demand. That is invisible in
  ordinary code but fatal in a scanner that asks for it per character — it made
  the self-hosted compiler quadratic in its input (15 s for a 6,400-function
  file). The fix is to hoist the length into a field (`Lexer.srcLen`) and build
  token literals byte by byte instead of slicing the source: ~70x faster, and
  the emitted C does not change. See `selfhost/README.md`.

## Roadmap next steps

Arenas (§6), the differential-testing interpreter (§10), the formatter and the
language server (§11) are all in place: `make diff` checks the interpreter and
compiled binary agree on every example, and `make fmt-check` verifies they are
canonically formatted. The one roadmap item left (spec §15) is self-hosting:
growing Brick until the compiler can be rewritten in Brick itself. See
`docs/SPEC.md` for the full plan.

Calling into C works end to end: when a program declares `extern` functions,
the compiler links the C sources named after the program, never a blanket glob:

| Program | Links |
| --- | --- |
| `foo.brick` | `foo.c` (if present) |
| `foo.brick` | `foo.d/*.c` (if the directory exists) |

So `examples/extern.brick` calls `triple` from `examples/extern.c` and builds
with `brick run`, and a program can split its C across `foo.d/`. Naming the C
file after the program keeps builds reproducible — an unrelated `.c` in the same
directory is never picked up.
