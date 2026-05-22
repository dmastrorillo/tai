package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/installer"
	"github.com/urfave/cli/v3"
)

const (
	commandsDirFlag = "commands-dir"
	forceFlag       = "force"
)

// newInstallCommand wires the `tai install` subcommand. It is
// repo-independent: the Action does NOT call RequireRepo and never
// touches the SQLite data directory. The subcommand surfaces three
// errors via the foundation contract: INSTALL_INVALID_TARGET (bad
// --commands-dir), INSTALL_TARGET_UNWRITABLE (cannot create or write
// to the target), INSTALL_LEDGER_CORRUPT (embedded ledger malformed).
func newInstallCommand(bundle installer.Bundle) *cli.Command {
	return &cli.Command{
		Name:  "install",
		Usage: "Write the bundled tai slash-commands into ~/.claude/commands/tai/",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  commandsDirFlag,
				Usage: "Override the target directory (default ~/.claude/commands/tai/)",
			},
			&cli.BoolFlag{
				Name:  forceFlag,
				Usage: "Overwrite user-modified files without prompting",
			},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			return runInstall(c, bundle)
		},
	}
}

// newUninstallCommand wires the `tai uninstall` subcommand. It is the
// symmetric verb to `tai install` and shares the same error surface.
func newUninstallCommand(bundle installer.Bundle) *cli.Command {
	return &cli.Command{
		Name:  "uninstall",
		Usage: "Remove the bundled tai slash-commands from ~/.claude/commands/tai/",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  commandsDirFlag,
				Usage: "Override the target directory (default ~/.claude/commands/tai/)",
			},
			&cli.BoolFlag{
				Name:  forceFlag,
				Usage: "Remove user-modified files without prompting",
			},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			return runUninstall(c, bundle)
		},
	}
}

func runInstall(c *cli.Command, bundle installer.Bundle) error {
	opts, err := resolveOptions(c)
	if err != nil {
		return err
	}
	opts.Bundle = bundle
	results, err := installer.Install(opts)
	if err != nil {
		return err
	}
	_, _ = io.WriteString(c.Writer, installer.FormatSummary(results, true))
	return nil
}

func runUninstall(c *cli.Command, bundle installer.Bundle) error {
	opts, err := resolveOptions(c)
	if err != nil {
		return err
	}
	opts.Bundle = bundle
	results, err := installer.Uninstall(opts)
	if err != nil {
		return err
	}
	_, _ = io.WriteString(c.Writer, installer.FormatSummary(results, false))
	return nil
}

// resolveOptions builds an installer.Options from the cli.Command
// flags, the user's home directory, and the current stdin state.
//
// The --commands-dir flag is treated as authoritative when provided.
// An explicit empty string fails INSTALL_INVALID_TARGET; absent flag
// falls back to ~/.claude/commands/tai/.
func resolveOptions(c *cli.Command) (installer.Options, error) {
	dir, err := resolveTargetDir(c)
	if err != nil {
		return installer.Options{}, err
	}

	opts := installer.Options{
		TargetDir: dir,
		Force:     c.Bool(forceFlag),
		IsTTY:     stdinIsTTY(c.Reader),
		Stdin:     c.Reader,
		Stdout:    c.Writer,
	}
	return opts, nil
}

func resolveTargetDir(c *cli.Command) (string, error) {
	// urfave/cli's IsSet() distinguishes "user provided" from
	// "default value". When the user explicitly passes
	// `--commands-dir ""`, IsSet returns true and the value is empty —
	// that's INSTALL_INVALID_TARGET.
	if c.IsSet(commandsDirFlag) {
		v := c.String(commandsDirFlag)
		if v == "" {
			return "", errcode.New(errcode.InstallInvalidTarget,
				"--commands-dir must not be empty").
				WithHelp("pass an absolute path, e.g. --commands-dir /tmp/cmds")
		}
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errcode.Wrap(errcode.InstallInvalidTarget, err,
			"cannot resolve home directory for default --commands-dir").
			WithHelp("set $HOME, or pass --commands-dir explicitly")
	}
	return filepath.Join(home, ".claude", "commands", "tai"), nil
}

// stdinIsTTY reports whether reader is an *os.File backed by a
// terminal device. urfave/cli wires c.Reader to os.Stdin in production
// and to a strings.Reader in tests; only the *os.File path can be a
// TTY.
//
// The mask `ModeDevice|ModeCharDevice` is the canonical stdlib idiom
// for "this file descriptor is a terminal". Using just ModeCharDevice
// can produce false positives on certain platforms where character
// devices that are not terminals also set that bit; requiring both
// rules them out.
func stdinIsTTY(reader io.Reader) bool {
	f, ok := reader.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	const ttyMask = os.ModeDevice | os.ModeCharDevice
	return fi.Mode()&ttyMask == ttyMask
}
