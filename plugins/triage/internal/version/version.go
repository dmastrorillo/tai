// Package version exposes the triage plugin binary's version string.
//
// The value is set via -ldflags at build time; it defaults to "dev" for
// local builds so unbuilt-from-source binaries are recognisable.
//
//	go build -ldflags="-X github.com/dmastrorillo/tai/plugins/triage/internal/version.String=v0.1.0"
//
// String is a package-level variable (not a constant) because the Go
// linker can only inject values into vars. This is the project's sole
// documented exception to CLAUDE.md's "no package-level mutable state"
// rule. Tests MUST NOT mutate String — assertions should read its
// current value, never overwrite it, to keep parallel tests race-free.
//
// The triage plugin has its own version package so the linker can
// inject a value distinct from core's. Both first-party binaries
// ship from this repo together, but the prefix-aware tag scheme
// owned by the `release-cycle` capability (`plugins/triage/vX.Y.Z`
// for this binary, bare `vX.Y.Z` for core) lets each release on
// its own schedule when needed.
package version

// String is the version string surfaced by the triage plugin's
// install/uninstall summary banners.
//
// Mutation is reserved for the linker (-ldflags -X). Do not assign to
// String from Go code, including tests.
var String = "dev"
