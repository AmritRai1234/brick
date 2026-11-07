#!/usr/bin/env python3
"""Build the four bench programs and time them.

Every program does the same three workloads with the same sizes and prints the
same checksum, so the numbers are comparable. Each binary is run several times
per workload; the best (minimum) wall time is reported, which is the least
noisy summary for a short CPU-bound run.
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
        out[name] = {mode: run([path, mode], capture_output=True, text=True).stdout.strip()
                     for mode in MODES}
    return out


def time_one(path, mode):
    best = None
    for _ in range(RUNS):
        start = time.perf_counter()
        subprocess.run([path, mode], stdout=subprocess.DEVNULL, check=True)
        elapsed = time.perf_counter() - start
        best = elapsed if best is None else min(best, elapsed)
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
        times = {n: time_one(progs[n], mode) for n in names}
        fastest = min(times.values())
        row = f"{mode:8}" + "".join(f"{times[n]:10.3f}" for n in names)
        print(row)
        print(f"{'':8}" + "".join(f"{'x' + format(times[n] / fastest, '.2f'):>10}" for n in names))
    return 0


if __name__ == "__main__":
    sys.exit(main())
