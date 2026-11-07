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
      `bool`, `text`; arithmetic, comparison and logical operators
- [x] **3. `if` / `else` and `while`**
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
      `==` / `!=`). These are the pieces a lexer and parser are built from.
- [~] **7. Formatter and language server** — the formatter is done
      (`brick fmt`, `make fmt-check`): it normalizes indentation (four spaces per
      level) and strips trailing whitespace while preserving every comment. The
      language server (LSP) is not started.
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
internal/interp/     AST -> values (tree-walking interpreter, for diff testing)
internal/format/     source -> canonically indented source (the formatter)
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

Arenas (§6), the differential-testing interpreter (§10) and the formatter (§11)
are all in place: `make diff` checks the interpreter and compiled binary agree on
every example, and `make fmt-check` verifies they are canonically formatted. What
remains from the roadmap (spec §15, §16) is the language server and self-hosting.
See `docs/SPEC.md` for the full plan.

Calling into C now works end to end: the compiler links any `.c` files that sit
next to the `.brick` source, so `examples/extern.brick` (which calls `triple`
from `examples/extern.c`) builds and runs with `brick run`.
