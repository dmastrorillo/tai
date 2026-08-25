// Plugin subprocess invocation: `tai <plugin> <args>` resolves to
// the plugin's installed binary, executes it with the wire-contract
// environment variables set, and passes stdin/stdout/stderr/exit
// through transparently.
//
// The Action in root.go calls dispatchPluginOrUnknown for any
// positional invocation that didn't match a reserved verb; the
// helper either execs the plugin and returns its exit code as a
// cli.ExitCoder, or surfaces UNKNOWN_SUBCOMMAND with plugin-aware
// "what to do" bullets.
//
// Spec: openspec/specs/plugin-host/spec.md
// §"Plugin subprocess invocation".

package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/datadir"
	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/pkg/taiplugin"
)

// pluginExitError is the cli.ExitCoder we return when a plugin
// subprocess exits non-zero. urfave/cli's runner reads ExitCode()
// and propagates it to the host process's exit code; the empty
// Error() string keeps cliout.WriteError from rendering a footer
// for a clean child failure (the plugin already wrote its own).
type pluginExitError struct{ code int }

func (e *pluginExitError) Error() string { return "" }
func (e *pluginExitError) ExitCode() int { return e.code }

// dispatchPluginOrUnknown is the catch-all routed from root.Action
// when a positional arg didn't match any reserved verb. It:
//
//  1. Looks the first arg up in plugins.json state.
//  2. Hit: execs the plugin's binary with the wire-contract env,
//     stdio attached, and returns its exit code as a cli.ExitCoder.
//  3. Miss: surfaces UNKNOWN_SUBCOMMAND with help bullets pointing
//     at `tai plugins list` and `tai plugins install <name>`.
//
// Returns the error verbatim (no wrapping) so the caller — the root
// Action — can `return` it straight through urfave/cli.
func dispatchPluginOrUnknown(ctx context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) == 0 {
		return cli.ShowAppHelp(c)
	}
	name := args[0]
	rest := args[1:]

	dataDir, derr := datadir.Resolve()
	if derr != nil {
		return derr
	}

	state, sErr := plugins.LoadState(dataDir)
	if sErr != nil {
		return sErr
	}
	if _, idx := state.Find(name); idx < 0 {
		return unknownSubcommandWithPluginHelp(name)
	}

	cfg, cfgErr := loadEffectiveConfig()
	if cfgErr != nil {
		return cfgErr
	}
	return execPlugin(ctx, name, rest, dataDir, cfg, c.Reader, c.Writer, c.ErrWriter)
}

// unknownSubcommandWithPluginHelp constructs the UNKNOWN_SUBCOMMAND
// error spec'd for the unresolved-plugin path. The help bullets
// point at `tai plugins list` and the install verb so the user
// knows where to look next.
//
// Special-cased verb: `update`. Per the update-banner spec, TAI is
// not self-updating — the banner names a package-manager command.
// The same applies when the user types `tai update` directly: the
// help bullets MUST name a package-manager command rather than the
// generic plugin-install boilerplate (TC-UB-006).
func unknownSubcommandWithPluginHelp(name string) error {
	if name == "update" {
		return errcode.New(errcode.UnknownSubcommand,
			"tai is not self-updating").
			WithHelp(
				"update via your package manager: `brew upgrade tai`",
				"or via the Go toolchain: `go install github.com/dmastrorillo/tai/core/cmd/tai@latest`",
				"to update a plugin, run `tai plugins update <name>`",
			)
	}
	return errcode.Newf(errcode.UnknownSubcommand,
		"unknown command: %q", name).
		WithHelp(
			"run `tai plugins list` to see installed plugins",
			"or install one: `tai plugins install "+name+"`",
			"`tai --help` lists every built-in verb",
		)
}

// execPlugin runs the installed plugin binary with the wire
// contract set. Stdio is wired through to the parent process; the
// child's exit code is returned as a *pluginExitError so urfave/cli
// surfaces the same code to the OS.
func execPlugin(ctx context.Context, name string, args []string, dataDir string, cfg *config.File, stdin io.Reader, stdout, stderr io.Writer) error {
	binPath := plugins.PluginBinaryPath(dataDir, name)
	if _, err := os.Stat(binPath); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"plugin %s is recorded in state but its binary is missing at %s", name, binPath).
			WithHelp(
				"reinstall the plugin: `tai plugins install "+name+"`",
				"or remove the stale state entry: `tai plugins remove "+name+"`",
			)
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = mergeEnv(os.Environ(), pluginEnv(dataDir, cfg))

	err := cmd.Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &pluginExitError{code: exitErr.ExitCode()}
	}
	return errcode.Wrapf(errcode.InternalError, err,
		"exec plugin %s: %s", name, err)
}

// pluginEnv returns the three wire-contract env vars (TAI_DATA_DIR,
// TAI_CLONE_DIR, TAI_TARGETS) as a KEY=VALUE slice. CloneDir is
// empty when no repo-url is configured.
func pluginEnv(dataDir string, cfg *config.File) []string {
	clone := ""
	if cfg != nil && strings.TrimSpace(cfg.RepoURL) != "" {
		clone = sync.CloneDir(dataDir)
	}
	targets := make([]taiplugin.Target, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		skills, commands, agents := t.EffectiveSubpaths()
		targets = append(targets, taiplugin.Target{
			Root:     t.Root,
			Skills:   skills,
			Commands: commands,
			Agents:   agents,
		})
	}
	env, err := taiplugin.EnvVars(dataDir, clone, targets)
	if err != nil {
		// EnvVars only errors if the JSON marshal of Target slice
		// fails — practically impossible. Fall back to a minimal
		// contract on the off chance it does, so the plugin still
		// sees the contract names.
		return []string{
			"TAI_DATA_DIR=" + dataDir,
			"TAI_CLONE_DIR=" + clone,
			"TAI_TARGETS=[]",
		}
	}
	return env
}

// mergeEnv overlays `over` on top of `base`, replacing same-key
// entries rather than appending. Used by execPlugin so the
// inherited environment is preserved EXCEPT for the TAI_*
// contract, which our overlay defines authoritatively.
func mergeEnv(base, over []string) []string {
	overKeys := map[string]string{}
	for _, kv := range over {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		overKeys[kv[:i]] = kv[i+1:]
	}
	out := make([]string, 0, len(base)+len(over))
	for _, kv := range base {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			out = append(out, kv)
			continue
		}
		key := kv[:i]
		if _, replace := overKeys[key]; replace {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range overKeys {
		out = append(out, k+"="+v)
	}
	return out
}
