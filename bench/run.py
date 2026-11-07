#!/usr/bin/env python3
"""Build the four bench programs and time them.

Every program does the same three workloads with the same sizes and prints the
same checksum, so the numbers are comparable.

Timing method: RUNS rounds, and inside each round all four programs run once per
workload, back to back. Interleaving keeps every language under the same machine
conditions (a busy background process otherwise inflates one language and not
the next), and the best (minimum) wall time over the rounds is reported, which
is the least noisy summary for a short CPU-bound run. Runs are pinned to one
core with taskset when it is available.
"""

import os
import shutil
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
OUT = os.path.join(HERE, "build")

CC = os.environ.get("CC", "cc")
CXX = os.environ.get("CXX", "c++")
RUSTC = os.environ.get("RUSTC", "rustc")

RUNS = int(os.environ.get("RUNS", "7"))

# Pin every run to one core. This machine's timings swing by 2x between runs
# when a process is free to migrate or share a core with something else; with
# taskset the numbers are repeatable to within a few percent. PIN="" disables it.
PIN = os.environ.get("PIN", "3")
TASKSET = shutil.which("taskset")
PIN_ARGS = ["taskset", "-c", PIN] if (TASKSET and PIN) else []
MODES = ["int", "sieve", "text"]


def run(cmd, **kw):
    return subprocess.run(cmd, check=True, **kw)


def build():
    shutil.rmtree(OUT, ignore_errors=True)
    os.makedirs(OUT)
    progs = {}
    progs["brick"] = os.path.join(OUT, "bench-brick")
    run([os.path.join(ROOT, "bin", "brick"), "build", os.path.join(HERE, "bench.brick"),
         "-o", progs["brick"]], stdout=subprocess.DEVNULL)
    progs["c"] = os.path.join(OUT, "bench-c")
    run([CC, "-std=c11", "-O2", "-o", progs["c"], os.path.join(HERE, "bench-c.c")])
    progs["cpp"] = os.path.join(OUT, "bench-cpp")
    run([CXX, "-std=c++17", "-O2", "-o", progs["cpp"], os.path.join(HERE, "bench-cpp.cpp")])
    progs["rust"] = os.path.join(OUT, "bench-rust")
    run([RUSTC, "-O", "-o", progs["rust"], os.path.join(HERE, "bench-rust.rs")])
    return progs


def checksums(progs):
    out = {}
    for name, path in progs.items():
        out[name] = {mode: run(PIN_ARGS + [path, mode], capture_output=True, text=True).stdout.strip()
                     for mode in MODES}
    return out


def time_all(progs, mode):
    """One timed run of every program per round, interleaved; keep the best."""
    best = {name: None for name in progs}
    for _ in range(RUNS):
        for name, path in progs.items():
            start = time.perf_counter()
            subprocess.run(PIN_ARGS + [path, mode], stdout=subprocess.DEVNULL, check=True)
            elapsed = time.perf_counter() - start
            if best[name] is None or elapsed < best[name]:
                best[name] = elapsed
    return best


def main():
    progs = build()
    sums = checksums(progs)
    names = ["brick", "c", "cpp", "rust"]

    print("checksums (they must agree):")
    for mode in MODES:
        values = {n: sums[n][mode] for n in names}
        agree = len(set(values.values())) == 1
        print(f"  {mode:6} {values}  {'ok' if agree else 'MISMATCH'}")
        if not agree:
            print("refusing to compare timings for a workload that disagrees", file=sys.stderr)
            return 1

    print(f"\nbest of {RUNS} runs, seconds (lower is better):")
    header = f"{'workload':8}" + "".join(f"{n:>10}" for n in names)
    print(header)
    print("-" * len(header))
    for mode in MODES:
        times = time_all(progs, mode)
        fastest = min(times.values())
        row = f"{mode:8}" + "".join(f"{times[n]:10.3f}" for n in names)
        print(row)
        print(f"{'':8}" + "".join(f"{'x' + format(times[n] / fastest, '.2f'):>10}" for n in names))
    return 0


if __name__ == "__main__":
    sys.exit(main())
