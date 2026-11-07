#!/usr/bin/env python3
"""Benchmark a very small HTTP server in Brick, C, Go and Rust.

All four servers do the same thing, with the same shape:

    accept -> one blocking read -> find the end of the request line
           -> one send -> close

Brick's socket syscalls come from a companion C file (server.c); everything
above them -- the accept loop, the request scan, the response -- is Brick.

The load generator is `ab` (ApacheBench): one process, many concurrent
connections, one request per connection (the servers answer `Connection: close`).

Method, same as bench/run.py: every server is checked first, then all four are
timed interleaved, round by round, and both the median rate and the per-request
CPU time (median and best round) are reported. The server
is pinned to one core and ab to another, because this machine is shared and
unpinned timings swing by 2-3x.

Usage:
    make bench-server            # or: python3 bench/server/run.py
    RUNS=5 CONNS=32 REQUESTS=20000 PIN=3 LOAD_PIN=4 python3 bench/server/run.py
"""

import os
import re
import shutil
import socket
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
BUILD = HERE / "build"
REPO = HERE.parent.parent

RUNS = int(os.environ.get("RUNS", "5"))
REQUESTS = int(os.environ.get("REQUESTS", "20000"))
CONNS = int(os.environ.get("CONNS", "32"))
PIN = os.environ.get("PIN", "3")
LOAD_PIN = os.environ.get("LOAD_PIN", "4")
BASE_PORT = int(os.environ.get("BASE_PORT", "18080"))

TASKSET = shutil.which("taskset")
AB = shutil.which("ab")


def pinned(core, argv):
    return [TASKSET, "-c", core] + argv if (TASKSET and core) else argv


def run_build():
    BUILD.mkdir(exist_ok=True)
    steps = [
        (["cc", "-std=c11", "-O2", "-Wall", "-Wextra", "-o", str(BUILD / "c-server"), str(HERE / "server-c.c")], "c"),
        (["go", "build", "-o", str(BUILD / "go-server"), str(HERE / "server-go.go")], "go"),
        (["rustc", "-O", "-o", str(BUILD / "rust-server"), str(HERE / "server-rust.rs")], "rust"),
        ([str(REPO / "bin" / "brick"), "build", str(HERE / "server.brick"), "-o", str(BUILD / "brick-server")], "brick"),
    ]
    for cmd, name in steps:
        print(f"  build {name:5s} {' '.join(cmd)}")
        result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        if result.returncode != 0:
            sys.exit(f"build failed for {name}:\n{result.stdout}")


def wait_for_port(port, timeout=5.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=0.2):
                return True
        except OSError:
            time.sleep(0.02)
    return False


def one_request(port, request, read_bytes=4096):
    with socket.create_connection(("127.0.0.1", port), timeout=2.0) as sock:
        sock.sendall(request)
        chunks = []
        while True:
            chunk = sock.recv(read_bytes)
            if not chunk:
                break
            chunks.append(chunk)
            if len(b"".join(chunks)) >= 15:
                break
    return b"".join(chunks)


class Server:
    def __init__(self, name, path, port):
        self.name = name
        self.path = path
        self.port = port
        self.proc = None

    def start(self):
        env = dict(os.environ, PORT=str(self.port))
        self.proc = subprocess.Popen(
            pinned(PIN, [str(self.path)]),
            env=env,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        if not wait_for_port(self.port):
            self.stop()
            sys.exit(f"{self.name}: did not start listening on port {self.port}")

    def stop(self):
        if self.proc is not None:
            try:
                os.killpg(os.getpgid(self.proc.pid), 15)
            except ProcessLookupError:
                pass
            self.proc.wait(timeout=5)
            self.proc = None


def check(server):
    """Every server must answer a GET with the body and a POST with 404."""
    server.start()
    try:
        get = one_request(server.port, b"GET / HTTP/1.0\r\n\r\n")
        post = one_request(server.port, b"POST / HTTP/1.0\r\n\r\n")
    finally:
        server.stop()
    if not get.startswith(b"HTTP/1.1 200") or not get.endswith(b"Hello, Brick!\n"):
        sys.exit(f"{server.name}: bad GET response:\n{get!r}")
    if not post.startswith(b"HTTP/1.1 404"):
        sys.exit(f"{server.name}: bad POST response:\n{post!r}")
    return True


AB_RPS = re.compile(r"Requests per second:\s+([0-9.]+)")
CLOCK_TICKS = os.sysconf("SC_CLK_TCK")


def cpu_seconds(pid):
    """utime + stime of one process, from /proc/<pid>/stat, in seconds."""
    with open(f"/proc/{pid}/stat") as handle:
        fields = handle.read().rsplit(") ", 1)[1].split()
    return (int(fields[11]) + int(fields[12])) / CLOCK_TICKS


def ab_round(server):
    """Run ab once against a running server; return (req/s, microseconds of
    server CPU per request). The CPU figure is the honest one on a shared box:
    it is the work the server did, not how long it waited for a core."""
    url = f"http://127.0.0.1:{server.port}/"
    cmd = pinned(LOAD_PIN, [AB, "-q", "-n", str(REQUESTS), "-c", str(CONNS), url])
    before = cpu_seconds(server.proc.pid)
    result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    after = cpu_seconds(server.proc.pid)
    match = AB_RPS.search(result.stdout)
    if match is None:
        sys.exit(f"ab failed against {server.name}:\n{result.stdout}")
    failed = re.search(r"Failed requests:\s+(\d+)", result.stdout)
    if failed and int(failed.group(1)) != 0:
        sys.exit(f"ab reported {failed.group(1)} failed requests against {server.name}")
    done = re.search(r"Complete requests:\s+(\d+)", result.stdout)
    served = int(done.group(1)) if done else REQUESTS
    return float(match.group(1)), (after - before) * 1e6 / served


def main():
    if AB is None:
        sys.exit("apachebench (ab) is required for the load")
    if not (REPO / "bin" / "brick").exists():
        sys.exit("bin/brick is missing: run `make build` first")

    print("building the four servers")
    run_build()

    names = ["brick", "c", "go", "rust"]
    servers = {name: Server(name, BUILD / f"{name}-server", BASE_PORT + i)
               for i, name in enumerate(names)}

    print("checking they all answer the same way")
    for name in names:
        check(servers[name])
        print(f"  {name:5s} ok")

    samples = {name: [] for name in names}
    cpus = {name: [] for name in names}
    print(f"\n{REQUESTS} requests, {CONNS} connections, {RUNS} interleaved rounds")
    for round_no in range(RUNS):
        for name in names:
            server = servers[name]
            server.start()
            try:
                rps, cpu_us = ab_round(server)
            finally:
                server.stop()
            samples[name].append(rps)
            cpus[name].append(cpu_us)
        print("  round %d: %s" % (round_no + 1,
                                  "  ".join(f"{n} {samples[n][-1]:,.0f}/s {cpus[n][-1]:.0f}us"
                                            for n in names)))

    def median(values):
        ordered = sorted(values)
        return ordered[len(ordered) // 2]

    med = {name: median(samples[name]) for name in names}
    med_cpu = {name: median(cpus[name]) for name in names}
    best_cpu = {name: min(cpus[name]) for name in names}
    fast = max(med.values())
    cheap = min(med_cpu.values())
    tight = min(best_cpu.values())
    print(f"\nrequests per second (higher is better; median of {RUNS} rounds)")
    print(f"{'server':8s} {'median':>9s} {'best':>9s} {'ratio':>7s} "
          f"{'cpu us/req':>11s} {'ratio':>7s} {'cpu best':>9s} {'ratio':>7s}")
    for name in names:
        print(f"{name:8s} {med[name]:9,.0f} {max(samples[name]):9,.0f} "
              f"{fast / med[name]:6.2f}x {med_cpu[name]:11.1f} {med_cpu[name] / cheap:6.2f}x "
              f"{best_cpu[name]:9.1f} {best_cpu[name] / tight:6.2f}x")
    print("\n'cpu us/req' is the server's own CPU time per request (utime+stime from\n"
          "/proc), which barely moves when the machine is busy; the rate swings\n"
          "with whatever else is running. 'cpu best' is the fastest round: on a\n"
          "shared machine contention (a busy SMT sibling, frequency scaling) can\n"
          "only add CPU time, so the minimum is the closest thing to a quiet box.")
    print("\nBrick's socket calls are the companion C file; everything above them is\n"
          "Brick. Single-threaded accept loops, so this measures per-request cost,\n"
          "not concurrency.")


if __name__ == "__main__":
    main()
