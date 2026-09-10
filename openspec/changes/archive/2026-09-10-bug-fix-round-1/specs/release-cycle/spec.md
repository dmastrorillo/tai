## ADDED Requirements

### Requirement: Pseudo-version detection collapses to `dev`

The version-resolution logic that runs at binary startup (in both `core/internal/version` and every plugin's `internal/version` package) SHALL detect Go-toolchain-synthesized pseudo-versions in `runtime/debug.ReadBuildInfo().Main.Version` and surface `dev` instead of the pseudo-version string.

A pseudo-version is the string Go generates when `go install` builds from a non-tagged commit (e.g. `@latest` against an untagged branch, or an explicit commit hash, or local source that was symlinked into the module cache). Its canonical form is:

```
vX.Y.Z(-<pre>)?[.-]\d{14}-[0-9a-f]{12}
```

Examples:

- `v0.1.2-0.20260609004251-72a773c77386` (no prerelease)
- `v0.0.0-20260609004251-72a773c77386` (canonical pre-1.0 form)
- `v0.2.0-rc.0.20260609004251-72a773c77386` (with prerelease)

When a binary's version-resolution logic receives a `Main.Version` matching this pattern, it SHALL return the linker-injected default (`dev`) instead of surfacing the pseudo-version string verbatim. Real tagged releases — bare `vX.Y.Z`, SemVer pre-releases (`vX.Y.Z-rc.1`, `vX.Y.Z-beta.2`), and the future `vX.Y.Z+meta` build-metadata shape — SHALL pass through unchanged.

Linker injection via `-ldflags "-X ..."` (the goreleaser path) continues to take precedence over both pseudo-version detection and BuildInfo fallback. Any non-`dev` linked value passes through verbatim.

#### Scenario: Pseudo-version surfaces as dev

- **WHEN** the binary is built via `go install github.com/dmastrorillo/tai/core/cmd/tai@<branch>` (or any non-tagged form that yields a pseudo-version in `Main.Version`)
- **THEN** `tai --version` prints `tai version dev`
- **AND** does NOT print the underlying pseudo-version string

#### Scenario: Clean tag surfaces as tag

- **WHEN** the binary is built via `go install github.com/dmastrorillo/tai/core/cmd/tai@v0.6.0` (with `Main.Version` exactly `v0.6.0`)
- **THEN** `tai --version` prints `tai version v0.6.0`

#### Scenario: Pre-release tag surfaces as pre-release tag

- **WHEN** the binary is built via `go install github.com/dmastrorillo/tai/core/cmd/tai@v0.6.0-rc.1` (with `Main.Version` exactly `v0.6.0-rc.1`)
- **THEN** `tai --version` prints `tai version v0.6.0-rc.1`
- **AND** the pseudo-version check does NOT match (the prerelease segment `rc.1` does not contain a 14-digit timestamp)

#### Scenario: Linker injection still wins for snapshot builds

- **WHEN** the binary is built via `make release-snapshot` (linker injects `String=v0.0.0-SNAPSHOT-<sha>`)
- **THEN** `tai --version` prints the linker-injected value verbatim
- **AND** the pseudo-version check is NOT consulted (the linker path bypasses BuildInfo entirely)

#### Scenario: Local `go build ./...` still surfaces dev

- **WHEN** the binary is built via plain `go build ./...` from a working tree (no ldflags, no module cache)
- **THEN** `tai --version` prints `tai version dev` (unchanged from existing behavior — `Main.Version` is `(devel)` in this case, already handled by the existing fallback)
