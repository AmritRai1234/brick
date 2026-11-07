# Working on Brick (notes for AI agents)

Brick is a small systems language that compiles to C
(`source → tokens → ast → checked → C → cc`). The Go compiler is the reference;
`selfhost/` is the same compiler rewritten in Brick and **must emit
byte-identical output**.

Full guide, including the symptom → cause debugging playbook:
`.agents/skills/brick-lang/SKILL.md` (load it as the `brick-lang` skill).
Quick gate: `.agents/skills/brick-lang/verify.sh` (add `deep` for the full run).

## Ground rules

1. Rebuild before trusting any error: `make build`. `./bin/brick` goes stale
   after Go edits and produces phantom `undefined function` errors.
2. Never change one implementation only. A change to emitted C, a diagnostic, or
   a type rule must be made in **both** `internal/` (Go) and `selfhost/`
   (Brick); the C runtime text is mirrored verbatim in `selfhost/runtime.brick`.
3. New runtime helpers must be gated on actual use (`programCalls` /
   `usesX` in `emitProgram`), so programs that do not use the feature emit
   exactly the C they did before.
4. Diagnostic strings are duplicated: `internal/checker/checker.go` and
   `selfhost/frontend.brick` must match word for word.
5. Verify with the ladder, in order: `go test ./...` → `make fmt-check` →
   `make diff` → `make selfhost` → `make selfhost-e2e` → `make bootstrap`.
   `make selfhost` names the component that diverged.
6. Before committing: bump `const version` in `cmd/brick/main.go`, add the README
   version-table row, `git tag -a vX.Y.Z`, and `make backup` (verified bundle in
   `../brick-backups/`).
7. Do not hand-write example layout; run `./bin/brick fmt examples/<file>.brick`.
8. When two implementations disagree and the spec does not say otherwise, the
   Go compiler wins; change the Brick side.

## Commands worth knowing

```sh
make build test vet fmt-check examples diff selfhost selfhost-e2e bootstrap bench backup
./bin/brick emit|check|errdump|typedump|tokdump|ast|fmt|interp|diff <file.brick>
BRICK_CC=clang ./bin/brick build prog.brick -o prog
```

`make bench` compares Brick, C, C++ and Rust on the same three workloads and
refuses to time a workload whose checksum disagrees. Timings are noisy on this
box; only its ratios are meaningful.
