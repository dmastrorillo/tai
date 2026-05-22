// Package verbs owns the canonical list of reserved top-level CLI
// verbs that the plugin host MUST refuse to install under.
//
// The authoritative list is maintained in
// openspec/changes/pivot-to-ai-as-code/specs/plugin-host/spec.md
// (Requirement: Plugin subprocess invocation). The Go code in this
// package mirrors that list; any addition or removal MUST happen in
// the same OpenSpec change that updates the spec.
//
// The plugin host uses Reserved() in two places:
//
//   - At plugin install time, to refuse PLUGIN_NAME_RESERVED when a
//     plugin's directory name collides with one of the verbs here.
//   - At command-dispatch time, to short-circuit known reserved verbs
//     before falling back to plugin-subprocess lookup. (A reserved
//     verb that isn't yet wired surfaces as UNKNOWN_SUBCOMMAND from
//     the root command's catch-all Action — it doesn't fall through
//     to the plugin path.)
package verbs

// reserved is the package-private store. Exposed via Reserved() so
// callers cannot mutate the slice.
var reserved = []string{
	"config",
	"sync",
	"repo",
	"install-commands",
	"workflow",
	"standards",
	"plugins",
	"help",
	"version",
}

// Reserved returns a copy of the canonical reserved-verbs list. The
// copy guards against accidental mutation by callers; the list is
// effectively immutable across a tai invocation.
//
// Order is the same as the spec's Requirement: Plugin subprocess
// invocation paragraph, for human-readable parity with the source of
// truth.
func Reserved() []string {
	out := make([]string, len(reserved))
	copy(out, reserved)
	return out
}

// IsReserved reports whether name (case-sensitive — the spec lists
// verbs in lowercase) is in the canonical reserved list. Plugin host
// install-time validation calls this to emit PLUGIN_NAME_RESERVED.
func IsReserved(name string) bool {
	for _, v := range reserved {
		if v == name {
			return true
		}
	}
	return false
}
