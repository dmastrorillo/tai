.PHONY: help build test test-race vet fmt-check lint check ledger-update release-snapshot release-core release-triage

# Default target prints the available recipes.
help:
	@echo "tai — available make targets:"
	@echo "  build              compile every package"
	@echo "  test               run unit + integration tests (no race detector)"
	@echo "  test-race          run tests with the Go race detector"
	@echo "  vet                go vet ./..."
	@echo "  fmt-check          fail if any file needs gofmt"
	@echo "  check              the full pre-merge sweep: fmt-check, vet, test, test-race"
	@echo "  ledger-update      recompute body hashes and append to commands/*.ledger.json"
	@echo "  release-snapshot   dry-run both goreleaser configs (no publish, no push); produces dist/"
	@echo "  release-core       publish a tai core release from the current vX.Y.Z tag"
	@echo "  release-triage     publish a triage plugin release from the current plugins/triage/vX.Y.Z tag"

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

# Dry-run both goreleaser configs. Produces archives in dist/ without
# publishing to GitHub Releases or pushing the brew formula. Run this
# before tagging to catch config drift early.
#
# Requires `goreleaser` (>= v1.13 for `monorepo.tag_prefix`) on PATH.
# See RELEASE.md for install instructions.
release-snapshot:
	goreleaser release --config .goreleaser.core.yaml --snapshot --clean --skip=publish,announce
	goreleaser release --config .goreleaser.triage.yaml --snapshot --clean --skip=publish,announce

# Publish a tai core release. The current HEAD MUST be at a bare
# vX.Y.Z tag (goreleaser refuses to run otherwise).
#
# Requires env:
#   GITHUB_TOKEN              — write scope on dmastrorillo/tai
#   HOMEBREW_TAP_GITHUB_TOKEN — write scope on dmastrorillo/homebrew-tap
release-core:
	goreleaser release --config .goreleaser.core.yaml --clean

# Publish a triage plugin release. The current HEAD MUST be at a
# `plugins/triage/vX.Y.Z` tag.
#
# Requires env:
#   GITHUB_TOKEN              — write scope on dmastrorillo/tai
# (No tap token required — plugins are not Homebrew-distributable.)
release-triage:
	goreleaser release --config .goreleaser.triage.yaml --clean
