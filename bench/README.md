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
make            # or: go build -o bin/brick ./cmd/brick
python3 bench/run.py
```

`run.py` builds all four with optimisation on (`-O2` for C/C++, `-O` for Rust,
Brick's default `-O2` C backend), verifies the checksums agree, then reports the
best of 7 wall-clock runs per workload. Set `RUNS=n` to change the sample count.

## Results

Best of 7 runs, seconds (lower is better); ratios are relative to the fastest
language in that row:

| workload | brick | c | c++ | rust |
| --- | --- | --- | --- | --- |
| int | 0.569 **(x1.00)** | 0.568 (x1.00) | 0.779 (x1.37) | 1.126 (x1.98) |
| sieve | 0.719 **(x1.00)** | 0.790 (x1.10) | 0.936 (x1.30) | 0.815 (x1.13) |
| text | 0.156 (x5.37) | 0.089 (x3.08) | 0.038 (x1.30) | 0.029 **(x1.00)** |

Readings:

- **`int`** — Brick and C tie at the top. Brick's C backend lowers the loop to
  plain `int64_t` arithmetic, so there is nothing to lose; Rust is slowest here
  because this benchmark uses release-mode overflow checks that Brick and C do
  not pay for.
- **`sieve`** — Brick is fastest. `List(bool)` element access goes through the
  generic `brick_list_at`, but the inner marking loop is memory-bound, so the
  extra indirection disappears into cache-miss latency.
- **`text`** — the real gap: Brick is ~5x slower than Rust. Each iteration does
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
      memcpy(builder->data + builder->len, text, strlen(text));  /* second strlen */
      builder->len += strlen(text);                              /* third strlen */
      ...
  }
  ```

  So every appended integer costs an arena allocation plus a `snprintf`, and
  every append walks the text three times. C's version formats straight into a
  stack buffer and `memcpy`s once; Rust and C++ reuse a growable `String`.

This is a known, fixable runtime cost rather than a codegen limitation: an
integer fast path in `brick_builder_add`, or a small non-allocating
`brick_int_to_text` (write into a stack buffer, return length), would remove
both the allocation and the repeated `strlen`.
