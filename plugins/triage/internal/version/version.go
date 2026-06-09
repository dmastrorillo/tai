// Package version exposes the triage plugin binary's version string.
//
// The value is set via -ldflags at build time; it defaults to "dev"
// for local builds so unbuilt-from-source binaries are recognisable.
//
//	go build -ldflags="-X github.com/dmastrorillo/tai/plugins/triage/internal/version.String=v0.1.0"
//
// String is a package-level variable (not a constant) because the Go
// linker can only inject values into vars. This is the project's sole
// documented exception to CLAUDE.md's "no package-level mutable state"
// rule. The init() block below writes to String exactly once at
// package-load time as a fallback for builds that do NOT run
// goreleaser and therefore don't carry the -ldflags injection: it
// reads the module's tagged version via
// `runtime/debug.ReadBuildInfo()` and uses that. Linker injection
// still wins when present. After init no further mutation happens;
// tests MUST NOT mutate String.
//
// The triage plugin has its own version package so the linker can
// inject a value distinct from core's. Both first-party binaries
// ship from this repo together, but the prefix-aware tag scheme
// owned by the `release-cycle` capability (`plugins/triage/vX.Y.Z`
// for this binary, bare `vX.Y.Z` for core) lets each release on its
// own schedule when needed.
//
// Note: the triage plugin is NOT distributable via `go install`
// (the plugin host requires binaries under <TAI_DATA_DIR>/plugins/
// <name>/), so the BuildInfo fallback in practice only fires for
// developers who run `go run plugins/triage/cmd/triage`. The
// production install path is `tai plugins install triage`, which
// downloads a goreleaser-built (ldflags-stamped) binary. The init
// fallback is kept symmetrical with the core package so both share
// the same shape.
package version

import "runtime/debug"

// String is the version string surfaced by the triage plugin's
// install/uninstall summary banners. Set EITHER via the linker
// (-ldflags -X — goreleaser path) OR via the init() block below
// (fallback for ad-hoc builds). After init it is read-only.
var String = "dev"

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	String = resolveVersion(String, info)
}

// resolveVersion picks the right version string given the linked
// default and the BuildInfo. Mirrors core/internal/version's helper
// so both packages share the same fallback semantics. See that
// package's doc comment for the rule table.
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
