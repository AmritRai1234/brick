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
| int | 0.595 (x1.03) | 0.576 **(x1.00)** | 0.575 **(x1.00)** | 1.092 (x1.90) |
| sieve | 0.824 (x1.05) | 0.981 (x1.25) | 0.787 **(x1.00)** | 0.786 **(x1.00)** |
| text | 0.078 (x3.85) | 0.042 (x2.08) | 0.021 (x1.02) | 0.020 **(x1.00)** |

What it shows:

- **`int`** — Brick is within 3% of C and C++: its C backend lowers the loop to
  plain `int64_t` arithmetic and the C compiler optimises it exactly as it
  optimises the hand-written C. Rust is 1.9x slower here even though the source
  is identical arithmetic. The difference is in the emitted code: GCC proves the
  intermediate value of `(x * 31 + i)` non-negative and emits a bare
  `mulq`/`shrq $29`, while LLVM adds a signed-remainder correction
  (`shr $63` / `sar $29` / `add`) on every iteration, and the loop is
  dependency-bound, so the longer chain costs ~2x.
- **`sieve`** — everyone lands within 25%, and Brick beats plain C. The marking
  loop is memory-bound, so Brick's `brick_list_at` call per element access
  disappears into cache-miss latency.
- **`text`** — the real gap: Brick is ~4x slower than C++ and Rust, ~2x slower
  than C. Each iteration is
  `brick_builder_add(&b, brick_int_to_text(i))`:

  ```c
  static inline const char *brick_int_to_text(int64_t value)
  {
      char *text = brick_alloc(32);      /* one allocation per integer */
      snprintf(text, 32, "%lld", (long long)value);
      return text;
  }

  static inline void brick_builder_add(BrickBuilder *builder, const char *text)
  {
      size_t need = builder->len + strlen(text) + 1;
      /* ...grow... */
      memcpy(builder->data + builder->len, text, strlen(text));  /* 2nd strlen */
      builder->len += strlen(text);                              /* 3rd strlen */
      ...
  }
  ```

  So every appended integer costs an arena allocation plus an `snprintf`, and
  every append walks the text three times. C formats straight into a stack
  buffer and `memcpy`s once; Rust and C++ reuse a growable `String` that never
  re-walks what it already holds.

This is a known, fixable runtime cost rather than a codegen limitation: an
integer fast path in `brick_builder_add` (write into a stack buffer, return the
length), or a non-allocating `brick_int_to_text`, would remove both the
allocation and the repeated `strlen`.

## A note on measurement

This machine is shared and noisy — a browser can steal a core mid-run and
inflate one language by 2-3x. Two things keep the numbers meaningful: every run
is pinned to one core (`taskset`), and each round runs all four programs back to
back per workload, so they all see the same conditions; the table reports the
best round. Absolute times still drift with system load; the ratios are stable.
