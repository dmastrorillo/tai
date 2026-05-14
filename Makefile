.PHONY: help build test test-race vet fmt-check lint check ledger-update

# Default target prints the available recipes.
help:
	@echo "tai — available make targets:"
	@echo "  build           compile every package"
	@echo "  test            run unit + integration tests (no race detector)"
	@echo "  test-race       run tests with the Go race detector"
	@echo "  vet             go vet ./..."
	@echo "  fmt-check       fail if any file needs gofmt"
	@echo "  check           the full pre-merge sweep: fmt-check, vet, test, test-race"
	@echo "  ledger-update   recompute body hashes and append to commands/*.ledger.json"

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt-check:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then \
		echo "gofmt would change these files:"; \
		echo "$$out"; \
		exit 1; \
	fi

check: fmt-check vet test test-race

# Recomputes each bundled command's body hash and appends it to the
# matching commands/<verb>.ledger.json. Idempotent — running it on an
# already-up-to-date tree is a no-op. Invoke this after editing any
# command markdown body, before committing.
ledger-update:
	go run ./cmd/tai-ledger
