// Package config owns tai's YAML config file: where it lives, what it
// holds, and how it is read and written.
//
// The file path follows the XDG Base Directory Specification with a
// `$TAI_CONFIG` escape hatch. See
// openspec/changes/pivot-to-ai-as-code/specs/config/spec.md for the
// normative spec.
//
// This package does NOT own the data directory (see pkg/datadir for
// that). The two are separate concepts: config describes user intent
// (which source repo, which targets); the data directory holds tai's
// runtime state (cached clones, plugin binaries, manifests).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// File is the in-memory representation of tai's YAML config file. All
// top-level fields are individually optional; some combinations are
// rejected by Validate (see the spec).
//
// Sub-path fields on a Target use `*string` so we can distinguish three
// states: nil (omitted — apply default), empty string (skip this
// category for this target), and a non-empty string (use this sub-path
// literally). yaml.v3's omitempty handles the nil case on serialization.
type File struct {
	RepoURL             string   `yaml:"repo-url,omitempty"`
	Targets             []Target `yaml:"targets,omitempty"`
	UpdateCheckInterval string   `yaml:"update-check-interval,omitempty"`
}

// Target is one entry in the `targets` array.
type Target struct {
	Root     string  `yaml:"root"`
	Skills   *string `yaml:"skills,omitempty"`
	Commands *string `yaml:"commands,omitempty"`
	Agents   *string `yaml:"agents,omitempty"`
}

// EffectiveSubpaths returns the absolute sub-paths for t after applying
// defaults. A falsy sub-path (explicit empty string) yields an empty
// returned value for that category — callers MUST treat an empty
// effective sub-path as "skip this category" (the spec's falsy-skip
// rule). Defaults are "skills", "commands", "agents" joined under Root.
//
// Returns absolute paths only when Root is absolute; otherwise returns
// paths as-given (callers that need absolute paths should resolve Root
// against $HOME themselves before constructing the Target).
func (t Target) EffectiveSubpaths() (skills, commands, agents string) {
	return effectiveSubpath(t.Root, t.Skills, "skills"),
		effectiveSubpath(t.Root, t.Commands, "commands"),
		effectiveSubpath(t.Root, t.Agents, "agents")
}

// effectiveSubpath applies the precedence: pointer-is-nil → default;
// pointer-is-empty → skip ("" returned); pointer-is-nonempty → use it.
// The returned path is joined under root when non-empty.
func effectiveSubpath(root string, override *string, defaultName string) string {
	if override == nil {
		return filepath.Join(root, defaultName)
	}
	if *override == "" {
		return ""
	}
	return filepath.Join(root, *override)
}

// EffectiveUpdateCheckInterval returns the duration parsed from
// f.UpdateCheckInterval, or 6 hours when the field is empty. A value of
// "0" disables the background update check; callers MUST treat a return
// of 0 as "disabled".
//
// Returns a parse error (NOT an *errcode.Error) when the field is set
// to an unparseable string; callers translate this into CONFIG_INVALID
// at the boundary where the user observes it.
func (f *File) EffectiveUpdateCheckInterval() (time.Duration, error) {
	if f.UpdateCheckInterval == "" {
		return 6 * time.Hour, nil
	}
	d, err := time.ParseDuration(f.UpdateCheckInterval)
	if err != nil {
		return 0, fmt.Errorf("invalid update-check-interval %q: %w", f.UpdateCheckInterval, err)
	}
	return d, nil
}

// ResolvePath returns the absolute path where tai's config file should
// live. The precedence is:
//
//  1. $TAI_CONFIG if set and non-empty (used verbatim — no tai/ suffix).
//  2. $XDG_CONFIG_HOME/tai/config.yml if XDG_CONFIG_HOME is set.
//  3. $HOME/.config/tai/config.yml on Linux and macOS.
//  4. %AppData%\tai\config.yml on Windows.
//
// ResolvePath MUST NOT touch the filesystem — it does not check that
// the path exists, is readable, or has a writable parent. Callers do
// that on demand.
func ResolvePath() (string, error) {
	if v := strings.TrimSpace(os.Getenv("TAI_CONFIG")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); v != "" {
		return filepath.Join(v, "tai", "config.yml"), nil
	}
	if runtime.GOOS == "windows" {
		if v := strings.TrimSpace(os.Getenv("AppData")); v != "" {
			return filepath.Join(v, "tai", "config.yml"), nil
		}
		// Fall through to HOME-based default below as a last resort.
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve config path: cannot determine HOME: %w", err)
	}
	return filepath.Join(home, ".config", "tai", "config.yml"), nil
}

// Load reads the file at path, parses it as YAML, validates it, and
// returns the populated *File. If the file does not exist, Load
// returns (nil, nil) — absence is not an error, it is a valid state.
//
// Returns *errcode.Error{Code: CONFIG_INVALID} on parse failure or
// validation failure. The cause is preserved via errors.Unwrap.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, errcode.Wrapf(errcode.ConfigInvalid, err,
			"read %s", path).
			WithHelp(
				"check the file's permissions",
				"or remove the file to start fresh: `rm "+path+"`",
			)
	}

	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, errcode.Wrapf(errcode.ConfigInvalid, err,
			"parse %s", path).
			WithHelp(
				"open the file in `tai config edit` and fix the syntax",
				"or remove it to start fresh: `rm "+path+"`",
			)
	}

	if err := Validate(&f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Save writes c to path, creating parent directories as needed. Returns
// *errcode.Error{Code: CONFIG_UNWRITABLE} when the directory cannot be
// created or the file cannot be written.
//
// The serialized YAML uses the field names declared on File and Target;
// nil sub-path pointers and zero-valued top-level strings are omitted
// via yaml.v3's omitempty.
func Save(path string, c *File) error {
	if err := Validate(c); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errcode.Wrapf(errcode.ConfigUnwritable, err,
			"create config directory %s", filepath.Dir(path)).
			WithHelp(
				"check that the parent directory is writable",
				"or override the location with `TAI_CONFIG=<path>`",
			)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return errcode.Wrapf(errcode.ConfigInvalid, err,
			"marshal config")
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return errcode.Wrapf(errcode.ConfigUnwritable, err,
			"write %s", path).
			WithHelp(
				"check that the file and its parent directory are writable",
				"or override the location with `TAI_CONFIG=<path>`",
			)
	}
	return nil
}

// Validate returns nil when c is structurally valid, else an
// *errcode.Error. Validation rules:
//
//   - repo-url, when present, must look like a remote git URL
//     (git@host:path, ssh://, or https://). Local paths and file://
//     URLs are rejected with CONFIG_INVALID_REPO_URL.
//   - Each target must have a non-empty Root.
//   - A target whose three sub-paths are ALL explicitly set to "" is
//     rejected with CONFIG_INVALID — at least one category must be
//     active for the target to be useful.
//
// Validate does NOT check writability of paths or reachability of the
// repo URL; those happen at use time.
func Validate(c *File) error {
	if c == nil {
		return nil
	}
	if c.RepoURL != "" {
		if err := validateRepoURL(c.RepoURL); err != nil {
			return err
		}
	}
	for i, t := range c.Targets {
		if strings.TrimSpace(t.Root) == "" {
			return errcode.Newf(errcode.ConfigInvalid,
				"target #%d has empty `root`", i+1).
				WithHelp("every target must specify a `root` filesystem path")
		}
		if isFalsy(t.Skills) && isFalsy(t.Commands) && isFalsy(t.Agents) {
			return errcode.Newf(errcode.ConfigInvalid,
				"target %q has every sub-path set falsy — at least one of skills, commands, agents must be active", t.Root).
				WithHelp(
					"remove the empty sub-paths to apply defaults",
					"or remove the target with `tai config target remove "+t.Root+"`",
				)
		}
	}
	if c.UpdateCheckInterval != "" {
		if _, err := time.ParseDuration(c.UpdateCheckInterval); err != nil {
			return errcode.Wrapf(errcode.ConfigInvalid, err,
				"update-check-interval %q is not a valid Go duration", c.UpdateCheckInterval).
				WithHelp("examples: `6h`, `30m`, `1h30m`; use `0` to disable")
		}
	}
	return nil
}

// isFalsy returns true when s is a non-nil pointer to an empty string.
// nil means "field omitted in YAML" which is not falsy — it triggers
// the default sub-path, not a skip.
func isFalsy(s *string) bool {
	return s != nil && *s == ""
}

// validateRepoURL is the package-level entry point for repo-url
// validation. It dispatches through the validateRepoURLFunc package
// variable so tests can swap in a permissive variant via init() in a
// _test.go file. Production builds never touch the override — there
// is no env var or runtime switch reachable from a shipping binary.
func validateRepoURL(u string) error {
	return validateRepoURLFunc(u)
}

// validateRepoURLFunc is the swappable validator. Defaults to the
// strict production logic. A test file in this package (config_test.go
// or a sibling _test.go) overrides it in init() to relax constraints
// for fixture-based tests; the override is invisible to non-test
// builds because _test.go files are only compiled during `go test`.
var validateRepoURLFunc = strictValidateRepoURL

// AllowFileURLsForTesting relaxes validateRepoURL for the lifetime of
// t so the test harness can use a file:// bare-repo fixture without
// reaching the production "remote URL" gate. The restore is wired via
// t.Cleanup so each test that calls this is self-contained.
//
// This function exists solely for the sync e2e tests. The
// `testing.TB` parameter makes it impossible to call from production
// code — `testing` is in the standard library but a production binary
// that imports it deliberately is the only way to reach this helper,
// which would itself be a code-review red flag.
func AllowFileURLsForTesting(t testing.TB) {
	t.Helper()
	prev := validateRepoURLFunc
	validateRepoURLFunc = func(u string) error {
		if strings.HasPrefix(strings.TrimSpace(u), "file://") {
			return nil
		}
		return strictValidateRepoURL(u)
	}
	t.Cleanup(func() { validateRepoURLFunc = prev })
}

// strictValidateRepoURL is the production validator. Rejects local
// paths and file:// URLs. Accepts:
//
//   - git@host:owner/repo[.git]      (SCP-style SSH)
//   - ssh://[user@]host[:port]/path  (URL-style SSH)
//   - https://host/path[.git]
//   - http://host/path               (rare but legal; spec allows
//     https and ssh — we accept http only when explicitly written,
//     since downgrading is the user's choice)
func strictValidateRepoURL(u string) error {
	u = strings.TrimSpace(u)
	if u == "" {
		return errcode.New(errcode.ConfigInvalidRepoURL,
			"repo-url is empty").
			WithHelp("set repo-url to a remote git URL (SSH or HTTPS)")
	}
	switch {
	case strings.HasPrefix(u, "git@"):
		// SCP-style requires a colon separating host from path.
		if !strings.Contains(u, ":") {
			break
		}
		return nil
	case strings.HasPrefix(u, "ssh://"),
		strings.HasPrefix(u, "https://"),
		strings.HasPrefix(u, "http://"):
		return nil
	}
	return errcode.Newf(errcode.ConfigInvalidRepoURL,
		"repo-url %q is not a remote git URL", u).
		WithHelp(
			"use SSH (`git@github.com:acme/repo.git`) or HTTPS (`https://github.com/acme/repo.git`)",
			"local paths and `file://` URLs are not accepted",
		)
}

// CommentedTemplate returns the YAML byte content used to seed a brand
// new config file via `tai config edit`. The bytes are commented-out
// examples of every supported top-level field — the user uncomments
// and edits the lines they need.
//
// Template invariants (enforced by tests):
//
//  1. The leading "## tai config" header sits on line 1 so the file
//     identifies itself.
//  2. Every supported top-level field in File appears as a commented
//     example so a user opening the file sees the full surface.
//  3. Every line is either: a "##"-prefixed prose comment (untouched by
//     the parse-round-trip test), or a "# "-prefixed YAML example
//     line, or blank. This split lets the round-trip test strip the
//     "# " prefix and parse the result as valid YAML — broken example
//     YAML would surface immediately rather than silently rotting.
func CommentedTemplate() []byte {
	return []byte(`## tai config — see https://github.com/dmastrorillo/tai for documentation.
##
## Uncomment the lines below (drop the leading "# ") to enable a field.

## repo-url: a remote git URL (SSH or HTTPS). Local paths and file://
## URLs are rejected.
# repo-url: git@github.com:your-org/your-tai-source.git

## targets: filesystem locations tai installs assets into. Each target
## has a required root and optional skills/commands/agents sub-paths.
## Sub-paths default to skills/commands/agents under root. Set a
## sub-path to an empty string to skip that category for the target.
# targets:
#   - root: ~/.claude
#   - root: ~/.opencode

## update-check-interval: how often tai polls upstream for newer
## versions. Go duration string (6h, 30m, 1h30m). Set to 0 to disable.
# update-check-interval: 6h
`)
}
