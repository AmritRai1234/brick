# A very small HTTP server, in four languages

The question: how many requests per second can a *tiny* HTTP/1.1 server handle,
and how much does the language underneath it cost?

Four servers, deliberately identical in shape:

| file | language | notes |
| --- | --- | --- |
| `server.brick` + `server.c` | Brick | sockets are `extern` C shims; the accept loop, request-line scan and response choice are Brick |
| `server-c.c` | C | the same code in plain C, same static buffer, same scan |
| `server-go.go` | Go | `net.Listen` + `net.Conn` — not `net/http`, that would be a different program |
| `server-rust.rs` | Rust | `TcpListener` + `TcpStream` |

Each one:

1. listens on `INADDR_LOOPBACK`, `SO_REUSEADDR`, backlog 512 (port from `$PORT`, default 18080);
2. accepts a connection, does **one** blocking `recv` into an 8192-byte static buffer;
3. scans for `\r` to find the request line;
4. answers `200 Hello, Brick!` if the line starts with `GET `, `404 nope` otherwise;
5. closes. One request per connection (`Connection: close`).

Nothing is pooled, buffered or threaded. That is the point: the benchmark should
measure the language's generated code plus the kernel, not library design.

## Run it

```sh
make bench-server
```

or, with knobs:

```sh
RUNS=7 REQUESTS=20000 CONNS=32 python3 bench/server/run.py
```

| env | default | meaning |
| --- | --- | --- |
| `RUNS` | 5 | rounds per server |
| `REQUESTS` | 20000 | requests per round |
| `CONNS` | 32 | concurrent connections for `ab` |
| `PIN` | 3 | CPU the *load generator* is pinned to |
| `LOAD_PIN` | 4 | CPU the *server* is pinned to |
| `BASE_PORT` | 18080 | first port handed out |

The harness builds all four, checks each one actually answers (`GET` → 200 +
`Hello, Brick!`, `POST` → 404), then runs them in interleaved rounds so a busy
moment on the machine hits every language roughly equally.

## What is measured, and why `cpu us/req`

Wall-clock req/s on a shared machine is not trustworthy: the same Brick binary
measured between 4,320 and 12,484 req/s across rounds while other processes
(chrome, a local LLM server) competed for cores. What *is* stable is the
server's own CPU time, read from `/proc/<pid>/stat` (`utime + stime`) before and
after each round and divided by the requests served.

So the headline numbers are **median req/s** and **median µs of server CPU per
request**. `ab` is pinned to one core and each server to another, so the load
generator never steals cycles from the server.

## Results

15,000 requests per round, 32 concurrent connections, 9 interleaved rounds,
median of the rounds (load average during the run: ~4.5–7, i.e. a busy box):

| server | req/s | ratio | cpu µs/req | `cpu best` | ratio |
| --- | ---: | ---: | ---: | ---: | ---: |
| Brick | 6,647 | 1.00 | 26.0 | 18.7 | 1.00 |
| C | 5,918 | 0.89 | 29.3 | 20.7 | 1.11 |
| Go | 8,125 | 1.22 | 48.0 | 30.0 | 1.61 |
| Rust | 6,521 | 0.98 | 26.7 | 24.0 | 1.29 |

The req/s column is the least meaningful one: the same C binary measured
between 4,551 and 12,620 req/s in different rounds while nothing about the
program changed. Across three separate multi-round sessions the medians landed
at Brick 24.0 / 20.0 / 26.0 µs per request and C 24.0 / 26.7 / 29.3 µs — the two
trade places run to run, which is the point: they are the same number.

## Reading it

**Brick is indistinguishable from C.** ~19–26 µs of server CPU per request for
both, depending on how contended the box was that minute. The generated C and
the hand-written C are the same program: one `accept`, one `recv`, a byte scan,
one `send`, one `close`. Brick's scanning loop
(`while i < n { if req[i] == 13 ... }`) compiles to the same pointer walk GCC
emits for the C version, so the abstraction costs nothing measurable here.

**Rust is ~1.1–1.3x the CPU per request.** The difference is in the standard
library's `TcpStream` path, not in the loop: `read`/`write_all` go through a
bounds-and-error-checked wrapper, and `position()` on a slice is a
well-optimised but distinct loop shape. Nothing pathological.

**Go spends 1.6–2.2x the CPU per request**, and on the busy box it also posted
the highest raw rate — it is the only implementation here that uses more than
one thread (the netpoller), so it wins wall-clock by cheating on core count
while doing more work per request. That is the runtime, not the code: every
connection is a goroutine with a `net.Conn` wrapper, and the netpoller has to be
entered and left per read and per write. With 32 connections in flight the CPU
cost hides behind the kernel — the throughput is fine — but the server is doing
more work for the same answer. A `net/http` version would add header parsing and
`bufio` on top of this.

**All four are limited by the same thing**: `accept` + `close` per request and
`ab`'s own loop. 6.5k req/s here is ~150 µs of wall time per request at 32-way
concurrency; the CPU sits at ~20–26 µs. This is a connection-rate benchmark as
much as a language one, which is why the four languages land so close together.

## Reproducing the isolation check

To confirm the Brick/C tie is not an artifact of the C shims, `requestLine(req)`
was temporarily replaced by a constant in a throwaway build, and a 7-round
interleaved CPU-only run gave Brick 37.0 µs/req (38 26 39 37 34 32 39) against C
36.5 µs/req (42 36 36 38 34 24 36) — identical within noise. The socket shims do
not move the number; everything above `srv_*` is Brick.

## Files

- `run.py` — build, correctness check, interleaved timed rounds, `/proc` CPU sampling
- `server.c` — the six socket shims linked into the Brick server (`srv_port`,
  `srv_listen`, `srv_accept`, `srv_read`, `srv_send`, `srv_close`)
- `build/` — compiled binaries and logs (gitignored)
