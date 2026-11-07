# Brick: Design Notes and Working Spec

*Draft 0.1. A personal project: a small, fast, readable systems language.*

## 1. Vision

Brick is a general-purpose, bare-metal programming language with three goals:

- **Small.** The whole language should fit in your head and in about 100 pages of spec. Someone should learn the basics in a day and be productive in a week.
- **Fast.** Performance close to C, because Brick compiles to C and lets a mature compiler (gcc or clang) do the optimizing.
- **Trustworthy.** Memory bugs such as use-after-free, dangling pointers and buffer overflows are ruled out by design, without a garbage collector and without lifetime annotations.

The name comes from the Lego idea: small, solid pieces that snap together. Big programs are built by combining small components, and most of what people use day to day is a library written in Brick itself.

Brick is built for one person first. That is a feature: with no users depending on it, the syntax can change whenever a better idea shows up.

## 2. Why not just use C, Rust or Zig?

- **C** is small and beautiful, and gives total control over memory layout. The price is that safety is entirely the programmer's job.
- **Rust** makes memory bugs impossible at compile time, but pays for it with a large set of concepts (ownership, borrowing, lifetimes, traits) that take months to learn.
- **Zig and Odin** are the closest to a nicer C, with explicit control and less ceremony, but they do not rule out memory bugs by design.
- **Goose** is a newer language exploring a memory-safe approach with no heap, no garbage collector and no lifetime annotations. It is a useful comparison for Brick's safety ideas. It is early, so its performance claims should be treated as promising until more people test them.

The core trade-off: you cannot fully have simple, fast and memory-safe without a garbage collector all at once. Rust gives up simplicity. C gives up safety. Brick gives up one specific freedom: **keeping long-lived references to data**. Everything else stays simple.

## 3. Design principles

1. **A tiny core, with friendly surface syntax on top.** The compiler only understands about a dozen core constructs. Every convenience must be explainable as a simple rewrite into the core (desugaring).
2. **One obvious way to do each thing.** Fewer choices means fewer mistakes, for humans and for language models.
3. **Explicit over implicit.** No implicit type conversions, no hidden control flow, no hidden allocation.
4. **No undefined behavior.** Every operation has a defined result.
5. **Local reasoning.** You can check a function by reading that function.
6. **Plain words over symbols for structure** (`function`, not `fn`), with ordinary math symbols where they read naturally.
7. **The compiler is dumb and predictable.** Each construct always turns into the same shape of output, so you can read the source and the output side by side.

## 4. The core language

Brick's bare minimum is about twelve pieces. Everything else is a library.

| # | Piece | Purpose |
| --- | --- | --- |
| 1 | Values: `int`, `float`, `bool`, `text` | Basic data |
| 2 | `var` | Name a value |
| 3 | `function` | Reusable code |
| 4 | `if` / `else` | Decisions |
| 5 | `while` | Repetition |
| 6 | `struct` | Group data into a type |
| 7 | `enum` and `match` | Choose between variants, which can carry data |
| 8 | Arrays | Fixed-size and slice collections |
| 9 | Borrowing (`&`, `&mut`) | Temporary access without copying |
| 10 | `arena` | Scoped memory |
| 11 | Errors as values | No exceptions |
| 12 | `import` | Modules |

Plus `return` and `extern` for calling C, which makes the keyword count roughly 20 in total.

### What is deliberately left out

Classes and inheritance, operator overloading, macros, header files, implicit conversions, garbage collection, null, exceptions, and undefined behavior.

## 5. Syntax sketch

This is an informal sketch, not a final grammar.

```
struct Point { x: int, y: int }

function move(p: &mut Point) {
    p.x = p.x + 1
    p.y = p.y + 1
}

function square(n: int) -> int {
    return n * n
}

function main() {
    var a = Point{ x: 1, y: 2 }
    move(&mut a)

    var total = 0
    var i = 1
    while i <= 5 {
        total = total + square(i)
        i = i + 1
    }
    print(total)
}
```

### Symbols

Source is typed in plain ASCII (`<=`, `->`, `!=`). An editor or font may display these as `≤`, `→`, `≠`, but the file stays easy to type and tool-friendly.

### Desugaring example

The surface `for` loop does not exist in the core. It is rewritten before compiling:

```
for item in list { print(item) }

// becomes, in the core:

var i = 0
while i < list.len {
    var item = list[i]
    print(item)
    i = i + 1
}
```

Rule for feature creep: a feature only enters Brick if it can be described as a simple rewrite into the core. If it cannot, it either becomes a new core construct (and you pay for that complexity knowingly) or it does not go in.

## 6. Memory model

This is the heart of Brick and the part most in need of real testing.

### Values are copied or moved

There is no hidden sharing. Assigning a value gives you your own copy, or moves it.

### Borrowing is temporary

A function may borrow data (`&` for read, `&mut` for write) for the duration of the call, but **cannot store the borrow**. This one rule removes the need for lifetime annotations.

### Arenas replace malloc and free

An `arena` block opens a region of memory. You allocate freely inside it, and everything is freed together when the block ends.

```
arena {
    var list = List(int)
    list.push(5)
}   // everything allocated inside is freed here
```

Allocation is just moving a pointer forward, and freeing is one operation, which is often faster than malloc and free. Data also ends up packed together, which CPUs like.

### Indices instead of pointers for linked data

Trees, graphs and similar structures use indices into an array instead of pointers. This is also how games and databases are often built for speed.

### No null

A value that might be absent uses an `Option`-style enum, and `match` forces you to handle both cases.

### What memory-safe means here

Gone by design: use-after-free, dangling pointers, buffer overflows (bounds checks on by default), null crashes, double frees.

Still possible: logic bugs. One new kind appears: a stale index can point at the wrong item without crashing. That is a logic bug, not memory corruption, but it still needs care.

## 7. Errors

There are no exceptions. A function that can fail returns a result value, and the caller must handle both outcomes.

```
enum Result { Ok(int), Err(text) }

function parse(s: text) -> Result { ... }

match parse("42") {
    Ok(n) => print(n)
    Err(msg) => print(msg)
}
```

## 8. Calling C

Because Brick compiles to C, calling existing C libraries is easy. You declare the function and the compiler emits a matching call.

```
extern function puts(s: text) -> int

function main() {
    puts("hello")
}
```

This means Brick starts from scratch as a language but inherits decades of C libraries (sockets, SQLite, OpenSSL, curl) from day one. Anything that crosses into C is outside Brick's safety guarantees, so that boundary should be small and clearly marked.

## 9. The compiler

### Pipeline

Text, then tokens (lexer), then a tree (parser), then checks (types and borrowing), then desugaring to the core, then output C.

Each stage is a small separate piece, which keeps the compiler easy to understand and debug.

### Implementation language: Go

Go is quick to write, compiles fast, produces a single binary, and its own type checker catches bugs in the compiler early. The language the compiler is written in does not decide whether Brick programs are safe. Safety comes from the checks the compiler performs and the code it emits.

### Parsing

Use a hand-written **recursive descent parser** rather than a generator such as Bison. For a small, regular syntax it is easier to debug and gives much better error messages.

### Output target

Compile to C first, then let gcc or clang handle machine code and optimization. This keeps the Brick compiler small, since optimizers are where most compiler complexity lives. Later options are LLVM or direct assembly.

### Bootstrapping

1. Write the first compiler in Go.
2. Grow Brick until it can express a compiler (structs, text, lists, files).
3. Rewrite the compiler in Brick, using compiler one to build compiler two.
4. Brick now compiles itself (self-hosting), and the Go version can be retired.

## 10. Keeping the compiler trustworthy

A compiler bug that lets something unsafe through is a bug in any language, so the aim is a compiler small enough to verify.

- **Fixed templates.** Each core construct always becomes the same output shape.
- **Readable output.** Open the generated C next to the source and read both.
- **Stage dump flag.** Print tokens, tree, core form and output so nothing is hidden.
- **Two implementations that must agree.** Write a slow, simple interpreter alongside the compiler. Run the same programs through both. Any difference means one has a bug (differential testing).
- **LLM as a second pair of eyes.** Ask a model whether a small Brick snippet and its output do the same thing. It is useful for spotting mismatches and writing test cases, but it can be wrong, so the tests and the interpreter are what decide.

Formal proof of a compiler exists (CompCert for C) but takes years of specialist work and is not realistic for a solo project.

## 11. Tooling

- **One command** to build, run and test.
- **One official formatter**, so all Brick code looks the same.
- **A language server (LSP)**, which reuses the lexer and parser already built for the compiler, giving editors autocomplete, error underlines, rename and go-to-definition.
- **Snippets first**, then structural search and replace that works on the code tree (finding repeated shapes with different values and offering to turn them into a function).
- **No invisible code-rewriting macros**, because tools cannot easily see what they produce.

## 12. Designing for AI as well as humans

Language models fail differently from people, so the language leans on:

- One way to do each thing, and explicit everything.
- Local reasoning, so no distant context is needed to check a function.
- Optional contracts (`requires` and `ensures`) that the compiler checks.
- Error messages that are also available as structured, machine-readable output with a suggested fix, so an AI can write, get precise feedback and fix.
- A fast compiler and a built-in test runner, because AI works by trial and error.
- A strict compiler. Humans find strictness annoying, but a model does not mind fixing twenty errors.

### The training-data problem

A brand-new language has no training data, so models will be weak at it at first. Mitigations: keep the syntax close to familiar C and Rust style, and keep the spec short enough to fit in a prompt along with examples.

### Natural language as the top layer

Keywords do not need to be cryptic, and plain words are fine. But the compiled layer must stay strict, because natural language is ambiguous and a compiler must give the same output for the same input every time.

The model: describe intent in plain language, an AI produces Brick, and the readable, strict Brick is the artifact that gets checked and trusted.

## 13. The patterns cookbook

Almost every program is built from about ten fundamental patterns. Brick should ship with a cookbook of roughly 30 tiny examples (each under 15 lines) covering them. It doubles as a first-day tutorial and as AI training material.

1. Store a value
2. Make a decision
3. Repeat
4. Group into a function
5. Group data into a type
6. Choose between variants
7. Collections (list, map, set)
8. Transform a collection (map, filter, sum)
9. Handle failure
10. Talk to the outside (files, network, time)

Common larger shapes to include: read input, process, write output; state machine (a `match` inside a loop); pipeline; cache; queue and worker. Many classic design patterns shrink to a function or disappear in a small language, such as iterator, strategy, observer and singleton.

## 14. Hypotheses: performance and safety

These are predictions to be tested, not results.

### Performance

- Close to C for most code, since gcc or clang does the optimizing.
- Arenas may beat malloc and free in many cases.
- Bounds checks might cost 0 to 15 percent depending on the code, with an opt-out for hot loops.
- Compile times should be fast because the language is small.

### Safety

- Strong on memory safety, as described in section 6.
- Threads are an open question. Without a borrowing-style rule for concurrency, data races remain possible.
- Any unsafe escape hatch or C call brings risks back in a small, visible area.

### Likely painful spots

- Parent pointers in trees, callbacks, and objects that reference each other.
- Long-lived shared data such as big caches. A per-request arena suits a server well, but a shared cache needs more thought.
- No libraries at first, which easy C interop softens.

### Best fit

Tools, servers, games and embedded work, where memory has a clear lifetime. A weaker fit for GUIs and heavily interconnected data.

## 15. Roadmap

Get a boring, working language first, then add the interesting parts one at a time.

1. **Hello world.** A Go program that reads a one-line Brick file and writes equivalent C.
2. **Variables and math.** `var x = 2 + 3` and `print(x)`.
3. **`if` and `while`.**
4. **`function`.**
5. **`struct`, then `extern`.**
6. **Borrowing, arenas, `enum` and `match`, errors.**
7. **Language server and formatter.**
8. **Interpreter for differential testing** (can start earlier).
9. **Self-hosting.**

Before and during this: write a few small programs in C (a word counter, a tiny parser) to feel what is annoying and what is worth keeping. Then write three small programs in early Brick (a text parser, a tiny web server, a game loop). Wherever it is annoying, the design is wrong, and that is cheap to find out early.

Suggested learning resource: *Crafting Interpreters* by Robert Nystrom, which is free online. For small-compiler inspiration, look at tcc and the Forth family.

## 16. Open questions

- Exactly how restrictive should "cannot store a borrow" be in practice?
- What is the concurrency story?
- How are generics handled? Compile-time code in the style of Zig is one candidate.
- How should arenas nest, and how does data escape an arena safely?
- What is the minimum standard library?
- Bounds checks: always on, or switchable per function?

## 17. Appendix: an operating system philosophy

A side thought, not part of the language. If designing an OS under today's constraints (memory cost, battery life), a good blend would be:

1. **Tiny kernel**, with drivers and the network stack running as ordinary programs, so a crashed driver just restarts.
2. **Capabilities** instead of ambient permissions: a program can only touch what it was explicitly handed.
3. **Idle by default**: no polling, and batched wakeups so the CPU sleeps as much as possible.
4. **Frugal memory**: compression instead of swapping, shared read-only code, cheap freezing of apps.
5. **Memory-safe core**, written in something like Rust or eventually Brick.
6. **Typed messages** between components instead of everything being text.

The hard part of a new OS is hardware support, not the kernel. For inspiration, look at seL4, Redox and Fuchsia.

## 18. Name notes

Runners-up considered: Flint, Arena, Pebble. For a personal project, name conflicts do not matter much, but searching registries is wise if it ever goes public.
