// `tai plugins` verb tree: install / update / remove / list. The
// subprocess hook that routes `tai <plugin> <args>` to the plugin
// binary lives in plugin_invoke.go to keep the CLI assembly here
// focused on the verb-tree shape.
//
// Spec: openspec/specs/plugin-host/spec.md

package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/pkg/datadir"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

func newPluginsCommand() *cli.Command {
	return &cli.Command{
		Name:  "plugins",
		Usage: "Manage installed tai plugins (list/install/update/remove)",

		Commands: []*cli.Command{
			newPluginsListCommand(),
			newPluginsInstallCommand(),
			newPluginsUpdateCommand(),
			newPluginsRemoveCommand(),
		},

		Action: func(_ context.Context, c *cli.Command) error {
			return cli.ShowSubcommandHelp(c)
		},
	}
}

// ─── tai plugins list ──────────────────────────────────────────────────

func newPluginsListCommand() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "List every installed plugin (name, version, install timestamp)",
		Action: runPluginsList,
	}
}

func runPluginsList(_ context.Context, c *cli.Command) error {
	dataDir, err := datadir.Resolve()
	if err != nil {
		return err
	}
	state, err := plugins.LoadState(dataDir)
	if err != nil {
		return err
	}
	return plugins.List(state, c.Writer)
}

// ─── tai plugins install <name> ────────────────────────────────────────

func newPluginsInstallCommand() *cli.Command {
	return &cli.Command{
		Name:      "install",
		Usage:     "Install a plugin from the built-in registry or an explicit source",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "source", Usage: "Explicit source: <host>/<org>/<repo>[/<subpath>]"},
			&cli.StringFlag{Name: "version", Usage: "Release tag to install (default: latest)"},
		},
		Action: runPluginsInstall,
	}
}

func runPluginsInstall(ctx context.Context, c *cli.Command) error {
	name, err := pluginsRequireName(c, "install")
	if err != nil {
		return err
	}
	cfg, dataDir, err := loadPluginsContext()
	if err != nil {
		return err
	}

	entry, err := plugins.Install(ctx, name, dataDir, cfg, plugins.InstallOptions{
		Source:  parseSourceFlag(c.String("source")),
		Version: c.String("version"),
		Stderr:  c.ErrWriter,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.Writer, "installed %s %s\n", entry.Name, entry.Version)
	return nil
}

// ─── tai plugins update <name> ─────────────────────────────────────────

func newPluginsUpdateCommand() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Re-fetch an installed plugin from its recorded source",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "version", Usage: "Release tag to update to (default: latest)"},
		},
		Action: runPluginsUpdate,
	}
}

func runPluginsUpdate(ctx context.Context, c *cli.Command) error {
	name, err := pluginsRequireName(c, "update")
	if err != nil {
		return err
	}
	cfg, dataDir, err := loadPluginsContext()
	if err != nil {
		return err
	}

	entry, err := plugins.Update(ctx, name, dataDir, cfg, plugins.UpdateOptions{
		Version: c.String("version"),
		Stderr:  c.ErrWriter,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.Writer, "updated %s to %s\n", entry.Name, entry.Version)
	return nil
}

// ─── tai plugins remove <name> ─────────────────────────────────────────

func newPluginsRemoveCommand() *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Usage:     "Uninstall a plugin (preserves the plugin's own runtime state)",
		ArgsUsage: "<name>",
		Action:    runPluginsRemove,
	}
}

func runPluginsRemove(_ context.Context, c *cli.Command) error {
	name, err := pluginsRequireName(c, "remove")
	if err != nil {
		return err
	}
	cfg, dataDir, err := loadPluginsContext()
	if err != nil {
		return err
	}
	res, err := plugins.Remove(name, dataDir, cfg, plugins.RemoveOptions{Stderr: c.ErrWriter})
	if err != nil {
		return err
	}
	if res.RetainedState != "" {
		_, _ = fmt.Fprintf(c.Writer, "removed %s (kept runtime state at %s)\n", name, res.RetainedState)
	} else {
		_, _ = fmt.Fprintf(c.Writer, "removed %s\n", name)
	}
	return nil
}

// ─── helpers ───────────────────────────────────────────────────────────

// pluginsRequireName extracts the plugin name from the command's
// first positional argument; emits MISSING_ARG when absent.
func pluginsRequireName(c *cli.Command, verb string) (string, error) {
	args := c.Args().Slice()
	if len(args) != 1 {
		return "", errcode.Newf(errcode.MissingArg,
			"tai plugins %s requires exactly one argument: <name>", verb).
			WithHelp("example: `tai plugins " + verb + " triage`")
	}
	return args[0], nil
}

// loadPluginsContext resolves config + data dir for the plugin verbs.
// Returns a non-nil *config.File even when no file exists on disk so
// callers can read Targets without a nil check.
func loadPluginsContext() (*config.File, string, error) {
	dataDir, err := datadir.Resolve()
	if err != nil {
		return nil, "", err
	}
	cfgPath, err := config.ResolvePath()
	if err != nil {
		return nil, "", errcode.Wrap(errcode.InternalError, err, err.Error())
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, "", err
	}
	if cfg == nil {
		cfg = &config.File{}
	}
	return cfg, dataDir, nil
}

// parseSourceFlag is a thin alias for plugins.ParseSource so the
// CLI layer reads naturally. Both `tai plugins install --source ...`
// and the `plugins.yml` auto-install path (in core/internal/sync)
// share the same parser.
func parseSourceFlag(raw string) plugins.Source { return plugins.ParseSource(raw) }
