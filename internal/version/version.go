// Package version exposes the tai binary's version string.
//
// The value is set via -ldflags at build time; it defaults to "dev" for
// local builds so unbuilt-from-source binaries are recognisable.
//
//	go build -ldflags="-X github.com/dmastrorillo/tai/internal/version.String=v0.1.0"
//
// String is a package-level variable (not a constant) because the Go
// linker can only inject values into vars. This is the project's sole
// documented exception to CLAUDE.md's "no package-level mutable state"
// rule. Tests MUST NOT mutate String — assertions should read its
// current value, never overwrite it, to keep parallel tests race-free.
package version

// String is the version string surfaced by `tai --version` and embedded
// in install/uninstall summary banners.
//
// Mutation is reserved for the linker (-ldflags -X). Do not assign to
// String from Go code, including tests.
var String = "dev"
