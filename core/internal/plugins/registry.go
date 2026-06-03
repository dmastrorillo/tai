// Package plugins owns the plugin-host machinery: the built-in
// first-party registry, the on-disk state, the fetch/install/update/
// remove/list verb implementations, and the asset-namespacing rules.
//
// See `openspec/changes/pivot-to-ai-as-code/specs/plugin-host/spec.md`
// for the normative behaviour; the package's exported API surface is
// shaped by the seven Requirements declared there.
package plugins

import (
	"strings"
	"testing"
)

// Source describes where to fetch a plugin's release asset from.
// The Host field is the release-host shortcode ("github.com" is the
// only supported value today); Repo is the `<org>/<repo>` slug. The
// fetcher uses (Host, Repo) to derive the Releases-API URL and the
// asset-name convention `tai-plugin-<plugin>-<os>-<arch>[.exe]`.
//
// Version is set per install: a registry entry leaves it empty
// ("latest" is implied) and `tai plugins <name> install --version`
// overrides it. Subpath is reserved for monorepo plugins whose
// release asset lives under a sub-directory; today no first-party
// plugin uses it.
type Source struct {
	Host    string
	Repo    string
	Subpath string
	Version string
}

// Empty reports whether s carries no fetch information. Helpers
// distinguish an unset Source (registry miss) from a populated one.
func (s Source) Empty() bool { return s.Host == "" && s.Repo == "" }

// builtin is the first-party plugin registry. Adding a new entry is
// part of the documented "add a first-party plugin" workflow
// (CLAUDE.md): land the entry here in the same OpenSpec change that
// introduces the plugin, then cut a release whose assets match
// `tai-plugin-<name>-<os>-<arch>`.
//
// Today this map intentionally has no entries — Phase 6 of
// pivot-to-ai-as-code adds `triage` once the triage migration lands
// (task 10.8). Keeping the map empty in Phase 4 lets every install
// test stage either explicit-source plugins or stub entries via
// `RegisterForTesting`; production builds never silently resolve a
// half-migrated `triage` from here.
var builtin = map[string]Source{}

// Lookup returns the registry entry for name, or (Source{}, false)
// when no entry exists. Callers that pass an explicit `--source`
// flag SHOULD prefer the flag over the registry; this function
// makes no decisions of its own.
func Lookup(name string) (Source, bool) {
	s, ok := builtin[name]
	return s, ok
}

// ParseSource splits `<host>/<org>/<repo>[/<subpath>]` into a Source.
// Empty input returns the zero Source so callers can pass it through
// to Install (which then falls back to the built-in registry). The
// parser is deliberately liberal so a future host (gitlab.com, etc.)
// does not require a parser change.
func ParseSource(raw string) Source {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Source{}
	}
	parts := strings.SplitN(raw, "/", 4)
	src := Source{Host: parts[0]}
	if len(parts) >= 3 {
		src.Repo = parts[1] + "/" + parts[2]
	}
	if len(parts) >= 4 {
		src.Subpath = parts[3]
	}
	return src
}

// RegisterForTesting injects a registry entry for the lifetime of t.
// Exists solely so tests can stage first-party plugin scenarios
// without seeding global state; production code never calls this
// because the registry is a compile-time constant.
//
// The `testing.TB` parameter is the standard guard pattern in this
// repo (matches `config.AllowFileURLsForTesting` and
// `sync.AutoInstallForTesting`): a production binary that imports
// `testing` is a code-review red flag, so RegisterForTesting can
// only be reached from tests.
func RegisterForTesting(t testing.TB, name string, src Source) {
	t.Helper()
	prev, had := builtin[name]
	builtin[name] = src
	t.Cleanup(func() {
		if had {
			builtin[name] = prev
		} else {
			delete(builtin, name)
		}
	})
}
