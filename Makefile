.PHONY: build test fmt fmt-check vet clean examples native diff interp selfhost backup

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

# Self-hosting (§15, roadmap item 9): build the Brick-written lexer and check
# its token dump is byte-identical to `brick tokdump` over every example.
selfhost: build
	@./bin/brick build selfhost/lexer.brick -o /tmp/brick-lex >/dev/null || exit 1
	@fail=0; \
	for f in examples/*.brick; do \
		if ! diff <(/tmp/brick-lex "$$f") <(./bin/brick tokdump "$$f"); then \
			echo "selfhost MISMATCH: $$f"; fail=1; \
		fi; \
	done; \
	if [ $$fail -ne 0 ]; then echo "self-hosting check FAILED"; exit 1; fi; \
	echo "selfhost ok: lexer.brick matches brick tokdump on every example"

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
