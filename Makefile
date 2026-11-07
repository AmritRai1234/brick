.PHONY: build test fmt vet clean examples native

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

# Run everything the native backend supports directly as machine code.
native: build
	@for f in examples/hello.brick examples/math.brick examples/fizzbuzz.brick; do \
		echo "=== $$f (native) ==="; \
		./bin/brick run --target native "$$f"; \
		echo; \
	done

clean:
	rm -rf bin
