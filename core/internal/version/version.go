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

import "runtime/debug"

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
	return v
}
