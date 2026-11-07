# Four-language benchmark: Brick vs C vs C++ vs Rust

The same three workloads, written in each language, timed head-to-head:

| workload | what it measures |
| --- | --- |
| `int` | a 200,000,000-iteration integer loop (`x = (x * 31 + i) % 1000000007`) |
| `sieve` | sieve of Eratosthenes to 50,000,000, counting primes |
| `text` | building a 1,000,000-entry comma-separated string with `Builder` / `String` |

Every implementation must produce the same checksum, so the timings compare
identical work:

- `int` → `381219692`
- `sieve` → `3001134`
- `text` → `6888890`

## Running it

```sh
make bench       # or: go build -o bin/brick ./cmd/brick && python3 bench/run.py
```

`run.py` builds all four with optimisation on (`-O2` for C/C++, `-O` for Rust,
Brick's default `-O2` C backend), verifies the checksums agree, then reports the
best of 12 interleaved rounds. `RUNS=n` changes the round count; `PIN=<core>`
changes the core they are pinned to, and `PIN=` turns pinning off.

## Results

Best of 12 interleaved rounds, seconds (lower is better); ratios are relative
to the fastest language in that row:

| workload | brick | c | c++ | rust |
| --- | --- | --- | --- | --- |
| int | 0.567 **(x1.00)** | 0.571 (x1.01) | 0.779 (x1.37) | 1.248 (x2.20) |
| sieve | 0.882 **(x1.00)** | 0.965 (x1.09) | 0.969 (x1.10) | 1.001 (x1.14) |
| text | 0.013 **(x1.00)** | 0.042 (x3.27) | 0.021 (x1.66) | 0.021 (x1.62) |

What it shows:

- **`int`** — Brick and C are level at the top: Brick's C backend lowers the
  loop to plain `int64_t` arithmetic and GCC optimises it exactly as it
  optimises the hand-written C. C++ and Rust are slower here even though the
  source is identical arithmetic. For Rust the difference is in the emitted
  code: GCC proves the intermediate value of `(x * 31 + i)` non-negative and
  emits a bare `mulq`/`shrq $29`, while LLVM adds a signed-remainder correction
  (`shr $63` / `sar $29` / `add`) on every iteration, and the loop is
  dependency-bound, so the longer chain costs ~2x.
- **`sieve`** — everyone lands within 15%. The marking loop is memory-bound, so
  Brick's `brick_list_at` call per element access disappears into cache-miss
  latency.
- **`text`** — Brick is now the fastest, 1.6x ahead of C++ and Rust and 3.3x
  ahead of C. Two runtime changes got it there (it was ~4x slower before):

  1. `brick_int_to_text` converts by hand into a small stack buffer and
     allocates exactly the digits it needs, instead of `brick_alloc(32)` plus
     `snprintf` per integer.
  2. `builder.add(to_text(n))` no longer builds an intermediate text at all.
     The emitter recognises that shape and calls `brick_builder_add_int`, which
     writes the digits straight into the builder's buffer — no allocation, no
     `strlen`. `brick_builder_add` itself now runs `strlen` once instead of
     three times, sharing its growth step with `brick_builder_reserve`.

  The C++ and Rust versions still construct a fresh `std::string` / `String` per
  number (`std::to_string(i)`, `i.to_string()`), which is exactly the
  allocation Brick now avoids.

So the summary is: Brick is at parity with C on compute-bound loops, and ahead
of all three on the text-building workload because the runtime knows the
difference between appending a string and appending an integer.

## A note on measurement

This machine is shared and noisy — a browser can steal a core mid-run and
inflate one language by 2-3x. Two things keep the numbers meaningful: every run
is pinned to one core (`taskset`), and each round runs all four programs back to
back per workload, so they all see the same conditions; the table reports the
best round. Absolute times still drift with system load; the ratios are stable.
