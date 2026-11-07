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
- [ ] 9. Self-hosting

### Semantics enforced by the checker

- No implicit conversions: `var x: float = 3` is an error, as is `1 + "two"`.
- `if`/`while` conditions must be `bool`; `&&`/`||`/`!` are bool-only.
- Functions and struct fields must be fully and correctly specified.
- Borrows are types (`&Point`), so a `&mut Point` cannot be passed where an
  `int` is expected, and vice versa.
- Only declarations are allowed at the top level; code lives in a function.

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
- `Builder()` (`add`, `build`) and `to_text(...)` for `int`, `float` and `bool`
  — assemble output without quadratic `+`
- top-level `const` — computed at compile time and inlined
- `arena { ... }` for bulk allocation
- output via `print` / `eprint`

With those in place, every box in the audit is ticked: Brick can express a
lexer, a parser, symbol tables, type checking and a C emitter. The roadmap's
last item (self-hosting, §15) is now a matter of writing the compiler in Brick
itself.

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
| `v0.3.8` | `to_text` for `float` and `bool` (current) |

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

## Roadmap next steps

Arenas (§6), the differential-testing interpreter (§10), the formatter and the
language server (§11) are all in place: `make diff` checks the interpreter and
compiled binary agree on every example, and `make fmt-check` verifies they are
canonically formatted. The one roadmap item left (spec §15) is self-hosting:
growing Brick until the compiler can be rewritten in Brick itself. See
`docs/SPEC.md` for the full plan.

Calling into C now works end to end: the compiler links any `.c` files that sit
next to the `.brick` source, so `examples/extern.brick` (which calls `triple`
from `examples/extern.c`) builds and runs with `brick run`.
