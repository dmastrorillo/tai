// Package taiplugin is the Go SDK for tai plugin authors.
//
// A plugin is a subprocess executable invoked by tai (see
// `openspec/specs/plugin-host/spec.md`). At invocation time, tai
// sets a small contract of environment variables that name the data
// directory, the source-repo clone path, and the configured targets
// — everything a plugin needs to know about the host's state without
// re-parsing tai's config.
//
// This package parses that contract into a typed [Context] so plugin
// authors don't reach for raw os.Getenv. For error output that looks
// identical to tai's own (footer template, exit-code mapping),
// plugins additionally import the sibling foundation packages
// directly: pkg/errcode to construct coded errors, pkg/cliexec to
// run the command tree and translate the result in main
// (`os.Exit(cliexec.Exit(os.Stderr, err))`), and pkg/cliout for any
// direct template rendering — the same wiring tai's own binaries
// use.
//
// Stability contract: every exported symbol in this package is on
// the same append-only stability contract as the rest of `pkg/`.
// Adding new fields to existing structs is allowed; renaming or
// removing them requires a major version bump of `tai`.
package taiplugin

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// envCloneDir, envTargets, envDataDir are the names of the three
// environment variables tai sets when invoking a plugin. They are
// part of the wire contract and MUST NOT be renamed without a major
// version bump.
const (
	envCloneDir = "TAI_CLONE_DIR"
	envTargets  = "TAI_TARGETS"
	envDataDir  = "TAI_DATA_DIR"
)

// Target mirrors the configured-target shape tai passes via
// TAI_TARGETS. All four fields hold absolute paths after tai has
// applied defaults — a plugin never needs to re-resolve a
// per-target sub-path. An empty Skills/Commands/Agents string means
// the user configured that category as falsy (skip).
type Target struct {
	// Root is the absolute path of the target's root (e.g.
	// `/home/dev/.claude`).
	Root string `json:"root"`

	// Skills is the absolute path of the target's skills sub-path,
	// or "" if the user configured this category as falsy.
	Skills string `json:"skills"`

	// Commands is the absolute path of the target's commands
	// sub-path, or "" if the user configured this category as falsy.
	Commands string `json:"commands"`

	// Agents is the absolute path of the target's agents sub-path,
	// or "" if the user configured this category as falsy.
	Agents string `json:"agents"`
}

// Context is the parsed wire contract a plugin receives at startup.
// Plugins call [Load] once to populate it; from then on every host
// fact is on this struct.
type Context struct {
	// DataDir is the absolute path of tai's data directory
	// (`<TAI_DATA_DIR>`). A plugin places its own per-user state
	// under `DataDir/plugins/<plugin-name>/state/` so tai's
	// remove command preserves it.
	DataDir string

	// CloneDir is the absolute path of tai's source-repo clone, or
	// "" if no `repo-url` is configured. Plugins that read from the
	// clone (e.g. to load shared workflows/standards) MUST handle
	// the empty case.
	CloneDir string

	// Targets is the list of configured targets in the order the
	// user authored them. Empty when no targets are configured —
	// most plugin operations should refuse to proceed in that case
	// with a [errcode.TaiNotConfigured] error.
	Targets []Target
}

// Load parses the wire-contract environment variables (TAI_DATA_DIR,
// TAI_CLONE_DIR, TAI_TARGETS) into a typed [Context]. Returns a
// `*errcode.Error{Code: INTERNAL_ERROR}` when TAI_TARGETS is set but
// fails to parse as JSON — that condition is a host bug, not a
// plugin-author bug.
//
// Load does NOT require any of the three variables to be set; an
// absent or empty variable yields the zero value of its field.
// Plugins that have hard requirements (e.g. "must have a clone") are
// expected to check the resulting Context fields and emit their own
// `errcode.TaiNotConfigured` if not satisfied.
func Load() (*Context, error) {
	ctx := &Context{
		DataDir:  strings.TrimSpace(os.Getenv(envDataDir)),
		CloneDir: strings.TrimSpace(os.Getenv(envCloneDir)),
	}

	raw := strings.TrimSpace(os.Getenv(envTargets))
	if raw == "" {
		return ctx, nil
	}
	if err := json.Unmarshal([]byte(raw), &ctx.Targets); err != nil {
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"parse %s as JSON array: %s", envTargets, err).
			WithHelp(
				"this is a host (tai) bug, not a plugin bug — please report it",
				"check that your `tai` and plugin versions are compatible",
			)
	}
	// Normalise JSON `null` (valid syntax that unmarshals to a nil
	// slice) to an empty slice so plugin authors can rely on
	// `len(ctx.Targets) == 0` AND `ctx.Targets != nil` consistently.
	if ctx.Targets == nil {
		ctx.Targets = []Target{}
	}
	return ctx, nil
}

// EnvVars returns the three wire-contract environment variables as a
// `KEY=VALUE` slice suitable for `exec.Cmd.Env`. Used by tai itself
// when invoking a plugin; exported so plugin-aware tooling (e.g.
// integration test harnesses) can reproduce the contract exactly.
//
// Errors are returned (rather than panicking) only if TAI_TARGETS
// fails to encode — practically impossible for the simple Target
// shape, but the API stays honest.
func EnvVars(dataDir, cloneDir string, targets []Target) ([]string, error) {
	targetsJSON := "[]"
	if len(targets) > 0 {
		b, err := json.Marshal(targets)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", envTargets, err)
		}
		targetsJSON = string(b)
	}
	return []string{
		envDataDir + "=" + dataDir,
		envCloneDir + "=" + cloneDir,
		envTargets + "=" + targetsJSON,
	}, nil
}
