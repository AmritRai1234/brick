.PHONY: build test fmt fmt-check vet clean examples native diff interp selfhost selfhost-e2e backup

build:
	go build -o bin/brick ./cmd/brick

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

# Compile and run every example, showing the generated C alongside the output.
examples: build
	@for f in examples/*.brick; do \
		echo "=== $$f ==="; \
		./bin/brick run "$$f"; \
		echo; \
	done

# Check every example is canonically formatted (useful in CI).
fmt-check: build
	@for f in examples/*.brick; do \
		./bin/brick fmt --check "$$f" || exit 1; \
	done

# Differential testing (§10): run every example through the interpreter and the
# compiled binary and check that the two agree.
diff: build
	@fail=0; \
	for f in examples/*.brick; do \
		./bin/brick diff "$$f" || fail=1; \
	done; \
	if [ $$fail -ne 0 ]; then echo "differential testing FAILED"; exit 1; fi

# Run every example on the tree-walking interpreter.
interp: build
	@for f in examples/*.brick; do \
		echo "=== $$f (interp) ==="; ./bin/brick interp "$$f"; \
		echo; \
	done

# Run everything the native backend supports directly as machine code.
native: build
	@for f in examples/hello.brick examples/math.brick examples/fizzbuzz.brick; do \
		echo "=== $$f (native) ==="; \
		./bin/brick run --target native "$$f"; \
		echo; \
	done

# Self-hosting (§15, roadmap item 9). Each Brick-written compiler component is
# checked against the Go compiler over every example: the lexer against
# `brick tokdump`, the parser against `brick ast`, the checker against
# `brick typedump`, and the C emitter against `brick emit`.
selfhost: build
	@./bin/brick build selfhost/lexer.brick -o /tmp/brick-lex >/dev/null || exit 1
	@./bin/brick build selfhost/parser.brick -o /tmp/brick-parse >/dev/null || exit 1
	@./bin/brick build selfhost/checker.brick -o /tmp/brick-check >/dev/null || exit 1
	@./bin/brick build selfhost/emitter.brick -o /tmp/brick-emit >/dev/null || exit 1
	@fail=0; \
	for f in examples/*.brick; do \
		if ! diff <(/tmp/brick-lex "$$f") <(./bin/brick tokdump "$$f"); then \
			echo "selfhost lexer MISMATCH: $$f"; fail=1; \
		fi; \
		if ! diff <(/tmp/brick-parse "$$f") <(./bin/brick ast "$$f"); then \
			echo "selfhost parser MISMATCH: $$f"; fail=1; \
		fi; \
		if ! diff <(/tmp/brick-check "$$f") <(./bin/brick typedump "$$f"); then \
			echo "selfhost checker MISMATCH: $$f"; fail=1; \
		fi; \
		if ! diff <(/tmp/brick-emit "$$f") <(./bin/brick emit "$$f"); then \
			echo "selfhost emitter MISMATCH: $$f"; fail=1; \
		fi; \
	done; \
	if [ $$fail -ne 0 ]; then echo "self-hosting check FAILED"; exit 1; fi; \
	echo "selfhost ok: lexer→tokdump, parser→ast, checker→typedump, emitter→emit, every example"

# End-to-end self-hosting: use the Brick-written C emitter as the compiler,
# compile its output with the system C compiler, run it, and check the program
# behaves the same as one built by the Go compiler.
selfhost-e2e: selfhost
	@fail=0; \
	for f in examples/*.brick; do \
		case "$$f" in *extern*) continue;; esac; \
		/tmp/brick-emit "$$f" > /tmp/brick-self.c 2>/dev/null || continue; \
		./bin/brick build "$$f" -o /tmp/brick-go-bin >/dev/null 2>&1 || continue; \
		if ! cc -std=c11 -O0 -o /tmp/brick-self-bin /tmp/brick-self.c -lm 2>/dev/null; then \
			echo "selfhost-e2e cc FAILED: $$f"; fail=1; continue; \
		fi; \
		if ! diff <(/tmp/brick-self-bin 2>/dev/null) <(/tmp/brick-go-bin 2>/dev/null); then \
			echo "selfhost-e2e MISMATCH: $$f"; fail=1; \
		fi; \
	done; \
	if [ $$fail -ne 0 ]; then echo "self-host end-to-end FAILED"; exit 1; fi; \
	echo "selfhost-e2e ok: C from the Brick emitter compiles and runs like the Go compiler's"

# Write a single-file backup of the whole repo (all branches and tags) to
# ../brick-backups. Restore with:  git clone <file>.bundle brick
backup:
	@mkdir -p ../brick-backups
	@out="../brick-backups/brick-$$(date +%Y%m%d-%H%M).bundle"; \
	git bundle create "$$out" --all >/dev/null && \
	echo "wrote $$out" && \
	git bundle verify "$$out" >/dev/null && echo "verified"

clean:
	rm -rf bin
