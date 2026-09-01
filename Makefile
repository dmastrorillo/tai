.PHONY: help build test test-race vet fmt-check lint check release-snapshot release-core release-triage

# Default target prints the available recipes.
help:
	@echo "tai — available make targets:"
	@echo "  build              compile every package"
	@echo "  test               run unit + integration tests (no race detector)"
	@echo "  test-race          run tests with the Go race detector"
	@echo "  vet                go vet ./..."
	@echo "  fmt-check          fail if any file needs gofmt"
	@echo "  check              the full pre-merge sweep: fmt-check, vet, test, test-race"
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

# Dry-run both goreleaser configs. Produces archives in dist/ without
# publishing to GitHub Releases or pushing the brew formula. Run this
# before tagging to catch config drift early.
#
# Each goreleaser invocation writes to its own subdirectory under
# dist/ (configured via `dist:` at the top of each .goreleaser.*.yaml)
# so the two runs don't clobber each other when --clean fires.
#
# Requires `goreleaser` v2 on PATH. See RELEASE.md for install
# instructions.
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
# `plugins/triage/vX.Y.Z` tag AND that tag MUST be pushed to
# origin (the pre-push check catches the common "forgot to push"
# mistake before goreleaser spends 10-30s building all archives).
#
# Two-step because goreleaser v2 OSS lacks `monorepo.tag_prefix`
# and `release.tag` (both Pro-only):
#   1. Run goreleaser with release: { disable: true } to BUILD
#      cross-platform archives + checksums under dist/triage/,
#      with GORELEASER_CURRENT_TAG=<bare semver> so .Version is
#      injected correctly into the binary.
#   2. Use `gh release create` to create the GitHub Release at the
#      full prefixed tag and upload the goreleaser-built artifacts.
#
# Requires env:
#   GITHUB_TOKEN  — write scope on dmastrorillo/tai (used by both
#                   goreleaser and gh).
# Requires `gh` CLI on PATH (https://cli.github.com/).
release-triage:
	@set -e; \
	tag="$$(git describe --exact-match --tags HEAD 2>/dev/null || true)"; \
	case "$$tag" in \
	  plugins/triage/v*) ;; \
	  *) echo "ERROR: HEAD not at a plugins/triage/vX.Y.Z tag (got: $$tag)" >&2; exit 1 ;; \
	esac; \
	if ! git ls-remote --exit-code origin "refs/tags/$$tag" >/dev/null 2>&1; then \
	  echo "ERROR: tag $$tag is not pushed to origin. Run: git push origin $$tag" >&2; \
	  exit 1; \
	fi; \
	bare="$${tag#plugins/triage/}"; \
	echo "Building triage release artifacts for $$tag (.Version=$$bare)..."; \
	GORELEASER_CURRENT_TAG="$$bare" \
	  goreleaser release --config .goreleaser.triage.yaml --clean --skip=validate; \
	echo "Creating GitHub Release $$tag..."; \
	case "$$bare" in \
	  *-*) gh release create "$$tag" --title "$$tag" --verify-tag --prerelease \
	         dist/triage/tai-plugin-triage-*.tar.gz dist/triage/checksums.txt ;; \
	  *)   gh release create "$$tag" --title "$$tag" --verify-tag \
	         dist/triage/tai-plugin-triage-*.tar.gz dist/triage/checksums.txt ;; \
	esac
