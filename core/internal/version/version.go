// Package version exposes the tai (core) binary's version string.
//
// The value is set via -ldflags at build time; it defaults to "dev"
// for local builds so unbuilt-from-source binaries are recognisable.
//
//	go build -ldflags="-X github.com/dmastrorillo/tai/core/internal/version.String=v0.1.0"
//
// String is a package-level variable (not a constant) because the Go
// linker can only inject values into vars. This is the project's sole
// documented exception to CLAUDE.md's "no package-level mutable state"
// rule. The init() block below writes to String exactly once at
// package-load time as a fallback for `go install
// github.com/dmastrorillo/tai/core/cmd/tai@vX.Y.Z` builds, which do
// NOT run goreleaser and therefore don't carry the -ldflags
// injection: it reads the module's tagged version via
// `runtime/debug.ReadBuildInfo()` and uses that. Linker injection
// still wins when present. After init no further mutation happens;
// tests MUST NOT mutate String.
//
// Each binary in this module has its own version package so the
// linker can inject distinct values per binary in the same build
// matrix; the triage plugin's binary has an independent copy under
// plugins/triage/internal/version. First-party binaries (core +
// plugins in this repo) ship from this repo together, but the
// prefix-aware tag scheme owned by the `release-cycle` capability —
// bare `vX.Y.Z` for core, `plugins/<name>/vX.Y.Z` for plugins —
// lets each release on its own schedule when needed.
package version

import (
	"regexp"
	"runtime/debug"
)

// String is the version string surfaced by `tai --version`. Set
// EITHER via the linker (-ldflags -X — goreleaser path) OR via the
// init() block below (go install path). After init it is read-only.
var String = "dev"

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	String = resolveVersion(String, info)
}

// pseudoVersionPattern matches Go-toolchain-synthesized pseudo-versions
// of the form `vX.Y.Z(-<prerelease>)?[.-]YYYYMMDDHHMMSS-<12hexsha>`.
// Examples:
//
//	v0.0.0-20260609004251-72a773c77386          (canonical pre-1.0)
//	v0.1.2-0.20260609004251-72a773c77386        (post-1.0 zero-prefix)
//	v0.2.0-rc.0.20260609004251-72a773c77386     (prerelease before timestamp)
//
// Real release tags — `v0.6.0`, `v0.6.0-rc.1`, `v0.6.0+meta` — do
// NOT match (no 14-digit timestamp + 12-hex sha tail).
var pseudoVersionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[A-Za-z0-9]+(\.[A-Za-z0-9]+)*)?[.-]\d{14}-[0-9a-f]{12}$`)

// looksLikePseudoVersion reports whether v matches Go's pseudo-version
// shape. Pseudo-versions appear when `go install` resolves a non-tagged
// commit (`@latest` on an untagged branch, an explicit commit hash, or
// a symlinked-local module). Surfacing them verbatim to users yields
// ugly strings like `v0.1.2-0.20260609004251-72a773c77386`; we collapse
// them to `dev` so locally-built binaries are visually distinct from
// real releases. Spec: openspec/specs/release-cycle/spec.md
// §"Pseudo-version detection collapses to `dev`" (TC-REL-009).
func looksLikePseudoVersion(v string) bool {
	return pseudoVersionPattern.MatchString(v)
}

// resolveVersion picks the right version string given the linked
// default and the BuildInfo. Extracted from init() so it can be
// table-tested without exercising the package-load path.
//
// Rules:
//   - Linker injection wins. If linked != "dev" the linker ran
//     successfully and we keep that value.
//   - "dev" + no BuildInfo: stay at "dev".
//   - "dev" + BuildInfo.Main.Version is empty or "(devel)": stay at
//     "dev" (local build, not installed via go install).
//   - "dev" + BuildInfo.Main.Version is a Go pseudo-version: stay at
//     "dev". Pseudo-versions are the `go install <branch-or-commit>`
//     and symlinked-local case; surfacing them verbatim is uglier
//     than the literal "dev" sentinel.
//   - "dev" + BuildInfo.Main.Version is a real semver tag (e.g.
//     "v0.1.0"): use it. This is the `go install ...@vX.Y.Z` case.
func resolveVersion(linked string, info *debug.BuildInfo) string {
	if linked != "dev" {
		return linked
	}
	if info == nil {
		return linked
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		return linked
	}
	if looksLikePseudoVersion(v) {
		return linked
	}
	return v
}
