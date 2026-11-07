# Notes: logic programming, and where it helps Brick

Source book (on this machine):
`~/Downloads/logical programing.djvu` —
**G. Metakides, A. Nerode, _Principles of Logic and Logic Programming_**
(Elsevier 1996; Russian translation, «Факториал», 1998). Extracted text:
`djvutxt "logical programing.djvu" out.txt`.

This is a logic textbook (propositional + predicate logic, semantic tableaux,
resolution) plus a Prolog section. Most of it is **not** needed to build a
systems-language compiler. Two pieces are genuinely valuable later.

## 1. Unification — for type inference (§2.9, §3.4.1)

Unification is the engine of Hindley–Milner type inference. Brick currently
checks types by a blunt rule: *two types must be exactly equal*. That is why
`var x: float = 3` is rejected with "no implicit conversion".

When Brick grows generics or inferred types, this is the algorithm to adopt
(Martelli–Montanari, the book's §2.9):

```
unify(t1, t2):
    f(a1..an) = f(b1..bn)   -> unify(a1,b1), ..., unify(an,bn)
    x = x                   -> nothing
    x = t                   -> bind x := t   (occurs-check: fail if x in t)
    f(...) = g(...)         -> FAIL (different functor)
```

Result is a **substitution** (a set of variable bindings). Applying it to the
whole program gives inferred types.

Concrete use in Brick later:

- A `List(T)` is already parameterised; unification would let `List(int)`
  arise from `List(xs[0])`-style inference instead of being written out.
- User generics: `function first(xs: List(T)) -> T` — unify `T` with the
  argument's element type at each call site.
- Keeps local reasoning (§5): each call unifies independently.

This would replace the "types must be equal" checks in
`internal/checker/checker.go` with a small unification pass.

## 2. Semantic tableaux / resolution — for contract checking (§1.7, §2.8, §1.9)

The spec (§12) wants **optional contracts** (`requires` / `ensures`) that the
compiler checks, and a compiler "small enough to verify". Tableaux and
resolution are the two proof methods the book develops (§1.7, §2.8) and proves
sound and complete (§1.10, §2.10). They are the natural foundation for a
future contract checker:

- `requires P` / `ensures Q` become logical goals to discharge.
- A tableau closes a goal when every branch contradicts — no external SMT
  solver required, keeping the compiler self-contained.
- Still a big project; record it as the long-term verification path, not a
  near-term task.

## 3. Backtracking + cut (§3.4.2) — a mental model only

Prolog's depth-first search with backtracking is the same shape as a
recursive-descent parser / pattern matcher. Useful as intuition for the
parser, but our compiler is deterministic, so nothing to implement here.

## Not useful for us

- Prolog syntax, cutting, expert systems, clause representation (§3.2–3.8).
  Brick is a systems language, not a logic language.
