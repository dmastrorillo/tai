// Config subcommand wiring for the core tai CLI.
//
// The `tai config` family is split per-verb so each Action stays small.
// Shared mechanics (resolving the config path, loading-or-bootstrapping,
// emitting the YAML, persisting) live in this file as package-private
// helpers used by the per-verb files.
//
// Every Action MUST return an error rather than calling os.Exit. The
// root command's main.go owns exit-code mapping via cliexec.Run.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// newConfigCommand builds the `tai config` subtree. Every verb in here
// reads or writes the YAML file at config.ResolvePath() (overridable
// via $TAI_CONFIG for tests and power users).
func newConfigCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Manage tai's YAML config file (repo-url, targets, update-check-interval)",

		Commands: []*cli.Command{
			newConfigShowCommand(),
			newConfigEditCommand(),
			newConfigSetCommand(),
			newConfigTargetCommand(),
		},

		Action: func(_ context.Context, c *cli.Command) error {
			// `tai config` with no subcommand: show help, exit 0.
			return cli.ShowSubcommandHelp(c)
		},
	}
}

// loadOrEmpty returns the parsed config, or an empty &config.File{}
// when no file exists on disk yet. The returned pointer is never nil
// on success — callers can read/mutate fields directly without a
// pre-write nil check. The no-write-on-read rule still holds: this
// function does NOT create the file on disk; that happens in
// config.Save when (and only when) a write subcommand persists.
func loadOrEmpty(path string) (*config.File, error) {
	f, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return &config.File{}, nil
	}
	return f, nil
}

// findTargetIndex returns the index of the target whose Root matches
// root exactly, or -1 when no such target exists. Used by both
// `target add` (to detect duplicates) and `target remove`.
func findTargetIndex(targets []config.Target, root string) int {
	for i, t := range targets {
		if t.Root == root {
			return i
		}
	}
	return -1
}

// ─── tai config show ───────────────────────────────────────────────────

func newConfigShowCommand() *cli.Command {
	return &cli.Command{
		Name:   "show",
		Usage:  "Print the current config as YAML",
		Action: runConfigShow,
	}
}

func runConfigShow(_ context.Context, c *cli.Command) error {
	path, err := config.ResolvePath()
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, err.Error())
	}

	f, err := config.Load(path)
	if err != nil {
		return err
	}

	if f == nil {
		// Absence of a config is not an error. Print an informational
		// message naming the next-step verbs to stdout — see TC-CONF-009
		// for the deliberate deviation from the general "stderr =
		// conversation" rule in specs/cli-framework/spec.md: a user
		// running `tai config show` is asking "what's in my config?",
		// and the answer ("nothing yet, do these things") is the data
		// product, so it belongs on stdout. Pipelines reading stdout
		// will receive this text instead of YAML when no config exists.
		// Build the blob in a Builder so the function has one ignored
		// Write boundary instead of four.
		var b strings.Builder
		fmt.Fprintf(&b, "No config file at %s.\n\n", path)
		b.WriteString("Next steps:\n")
		b.WriteString("  tai config target add <root>     # add a destination for tai to install assets into\n")
		b.WriteString("  tai config edit                   # open the config in $EDITOR\n")
		_, _ = io.WriteString(c.Writer, b.String())
		return nil
	}

	// yaml.Marshal emits canonical YAML, NOT the file's raw bytes —
	// comments and unusual key orderings on disk are lost in this
	// rendering. The spec promises "the YAML representation of the
	// current config" (TC-CONF-008), which a canonical rendering
	// satisfies. If byte-fidelity becomes a requirement, switch to
	// os.ReadFile + io.WriteString.
	data, err := yaml.Marshal(f)
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, "marshal config")
	}
	_, _ = c.Writer.Write(data)
	return nil
}

// ─── tai config edit ───────────────────────────────────────────────────

func newConfigEditCommand() *cli.Command {
	return &cli.Command{
		Name:   "edit",
		Usage:  "Open the config file in $EDITOR (creates a commented template on first call)",
		Action: runConfigEdit,
	}
}

func runConfigEdit(ctx context.Context, c *cli.Command) error {
	// strings.TrimSpace expands the spec's literal "EDITOR is unset"
	// trigger to also cover EDITOR set to whitespace-only — a user
	// invoking `EDITOR=" " tai config edit` would otherwise see an
	// opaque exec error; the explicit CONFIG_EDITOR_UNSET footer is
	// more actionable.
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return errcode.New(errcode.ConfigEditorUnset,
			"$EDITOR is not set").
			WithHelp(
				"set EDITOR to your editor of choice (`export EDITOR=vim`)",
				"or invoke the editor directly on the config file",
			)
	}

	path, err := config.ResolvePath()
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, err.Error())
	}

	// Bootstrap the commented template if the file doesn't exist yet.
	// errors.Is/fs.ErrNotExist (Go 1.13+) is preferred over os.IsNotExist
	// because it unwraps error chains — wrappers from NFS / FUSE go
	// stacks would otherwise bypass the older predicate.
	if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errcode.Wrapf(errcode.ConfigUnwritable, err,
				"create config directory %s", filepath.Dir(path))
		}
		if err := os.WriteFile(path, config.CommentedTemplate(), 0o644); err != nil {
			return errcode.Wrapf(errcode.ConfigUnwritable, err,
				"write template to %s", path)
		}
	}

	// $EDITOR can be a multi-word command (e.g. `code --wait`). Split
	// on whitespace so the user's flags survive.
	//
	// Known limitation: editor paths containing whitespace (e.g.
	// `/Applications/Visual Studio Code.app/.../code`) are broken by
	// this splitter. The git CLI sidesteps it by running $EDITOR
	// through `sh -c`; we accept the limitation for now because the
	// affected paths are uncommon for headless tai use and the user
	// can always set EDITOR to a wrapper script.
	parts := strings.Fields(editor)
	args := append(parts[1:], path)
	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Stdin = c.Reader
	cmd.Stdout = c.Writer
	cmd.Stderr = c.ErrWriter

	if err := cmd.Run(); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"editor exited with error: %s", editor).
			WithHelp("check that $EDITOR points at an executable")
	}
	return nil
}

// ─── tai config set ────────────────────────────────────────────────────

func newConfigSetCommand() *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "Set a scalar top-level config key (repo-url, update-check-interval)",
		ArgsUsage: "<key> <value>",
		Action:    runConfigSet,
	}
}

func runConfigSet(_ context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) != 2 {
		return errcode.New(errcode.MissingArg,
			"tai config set requires exactly two arguments: <key> <value>").
			WithHelp(
				"example: `tai config set repo-url git@github.com:acme/repo.git`",
				"run `tai config set --help` to see the supported keys",
			)
	}
	key, value := args[0], args[1]

	// Reject nested/array keys with a precise code so the user knows
	// to use a dedicated subcommand or `tai config edit`.
	if strings.ContainsAny(key, ".[") {
		return errcode.Newf(errcode.ConfigKeyNotScriptable,
			"key %q is nested or indexed — `tai config set` only handles scalar top-level keys", key).
			WithHelp(
				"for targets, use `tai config target add/list/remove`",
				"for anything else, use `tai config edit` to edit the YAML directly",
			)
	}

	path, err := config.ResolvePath()
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, err.Error())
	}
	f, err := loadOrEmpty(path)
	if err != nil {
		return err
	}

	switch key {
	case "repo-url":
		f.RepoURL = value
	case "update-check-interval":
		f.UpdateCheckInterval = value
	default:
		return errcode.Newf(errcode.ConfigKeyNotScriptable,
			"unknown scalar key %q", key).
			WithHelp(
				"supported keys: `repo-url`, `update-check-interval`",
				"for targets, use `tai config target add/list/remove`",
			)
	}

	return config.Save(path, f)
}

// ─── tai config target ────────────────────────────────────────────────

func newConfigTargetCommand() *cli.Command {
	return &cli.Command{
		Name:  "target",
		Usage: "Manage the targets array (add/list/remove)",

		Commands: []*cli.Command{
			newConfigTargetAddCommand(),
			newConfigTargetListCommand(),
			newConfigTargetRemoveCommand(),
		},

		Action: func(_ context.Context, c *cli.Command) error {
			return cli.ShowSubcommandHelp(c)
		},
	}
}

func newConfigTargetAddCommand() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Append a new target to the config",
		ArgsUsage: "<root>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "skills", Usage: "Override the `skills` sub-path for this target"},
			&cli.StringFlag{Name: "commands", Usage: "Override the `commands` sub-path for this target"},
			&cli.StringFlag{Name: "agents", Usage: "Override the `agents` sub-path for this target"},
		},
		Action: runConfigTargetAdd,
	}
}

func runConfigTargetAdd(_ context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) != 1 {
		return errcode.New(errcode.MissingArg,
			"tai config target add requires exactly one argument: <root>").
			WithHelp("example: `tai config target add ~/.claude`")
	}
	root := args[0]

	path, err := config.ResolvePath()
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, err.Error())
	}
	f, err := loadOrEmpty(path)
	if err != nil {
		return err
	}

	if findTargetIndex(f.Targets, root) >= 0 {
		return errcode.Newf(errcode.ConfigDuplicateTarget,
			"target with root %q already exists", root).
			WithHelp(
				"remove the existing target first: `tai config target remove "+root+"`",
				"or edit the config directly: `tai config edit`",
			)
	}

	tgt := config.Target{Root: root}
	if c.IsSet("skills") {
		v := c.String("skills")
		tgt.Skills = &v
	}
	if c.IsSet("commands") {
		v := c.String("commands")
		tgt.Commands = &v
	}
	if c.IsSet("agents") {
		v := c.String("agents")
		tgt.Agents = &v
	}
	f.Targets = append(f.Targets, tgt)

	return config.Save(path, f)
}

func newConfigTargetListCommand() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "List configured targets",
		Action: runConfigTargetList,
	}
}

func runConfigTargetList(_ context.Context, c *cli.Command) error {
	path, err := config.ResolvePath()
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, err.Error())
	}
	f, err := config.Load(path)
	if err != nil {
		return err
	}

	if f == nil || len(f.Targets) == 0 {
		_, _ = io.WriteString(c.Writer, "(no targets configured)\n")
		return nil
	}

	w := tabwriter.NewWriter(c.Writer, 0, 0, 2, ' ', 0)
	// tabwriter is buffered — per-line writes can't actually fail
	// against an in-memory buffer; the real I/O error surfaces from
	// w.Flush() below, where we DO capture it.
	_, _ = fmt.Fprintln(w, "root\tskills\tcommands\tagents")
	for _, t := range f.Targets {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			t.Root,
			subpathDisplay(t.Skills, "skills"),
			subpathDisplay(t.Commands, "commands"),
			subpathDisplay(t.Agents, "agents"),
		)
	}
	return w.Flush()
}

// subpathDisplay renders a sub-path pointer for the table:
//   - nil  → "<default: skills>" / "<default: commands>" / etc. so the
//     user sees both the implicit-default state AND what it resolves to
//   - ""   → "(skip)"   so the falsy-skip case is visible
//   - else → the literal override
func subpathDisplay(p *string, defaultName string) string {
	if p == nil {
		return "<default: " + defaultName + ">"
	}
	if *p == "" {
		return "(skip)"
	}
	return *p
}

func newConfigTargetRemoveCommand() *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Usage:     "Remove the target whose root matches exactly",
		ArgsUsage: "<root>",
		Action:    runConfigTargetRemove,
	}
}

func runConfigTargetRemove(_ context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) != 1 {
		return errcode.New(errcode.MissingArg,
			"tai config target remove requires exactly one argument: <root>").
			WithHelp("example: `tai config target remove ~/.claude`")
	}
	root := args[0]

	path, err := config.ResolvePath()
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, err.Error())
	}
	f, err := loadOrEmpty(path)
	if err != nil {
		return err
	}

	idx := findTargetIndex(f.Targets, root)
	if idx < 0 {
		return errcode.Newf(errcode.ConfigTargetNotFound,
			"no target with root %q", root).
			WithHelp(
				"run `tai config target list` to see configured targets",
				"or use `tai config edit` to inspect the YAML directly",
			)
	}

	f.Targets = append(f.Targets[:idx], f.Targets[idx+1:]...)
	return config.Save(path, f)
}
