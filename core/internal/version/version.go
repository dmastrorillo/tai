// Package version exposes the tai (core) binary's version string.
//
// The value is set via -ldflags at build time; it defaults to "dev" for
// local builds so unbuilt-from-source binaries are recognisable.
//
//	go build -ldflags="-X github.com/dmastrorillo/tai/core/internal/version.String=v0.1.0"
//
// String is a package-level variable (not a constant) because the Go
// linker can only inject values into vars. This is the project's sole
// documented exception to CLAUDE.md's "no package-level mutable state"
// rule. Tests MUST NOT mutate String — assertions should read its
// current value, never overwrite it, to keep parallel tests race-free.
//
// Each binary in this module has its own version package so the linker
// can inject distinct values per binary in the same build matrix; the
// triage plugin's binary has an independent copy under
// plugins/triage/internal/version. First-party binaries (core + plugins
// in this repo) ship from this repo together, but the prefix-aware tag
// scheme owned by the `release-cycle` capability — bare `vX.Y.Z` for
// core, `plugins/<name>/vX.Y.Z` for plugins — lets each release on
// its own schedule when needed.
package version

// String is the version string surfaced by `tai --version`.
//
// Mutation is reserved for the linker (-ldflags -X). Do not assign to
// String from Go code, including tests.
var String = "dev"
