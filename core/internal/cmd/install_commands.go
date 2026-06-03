// `tai install-commands` verb: ship TAI's bundled built-in slash
// commands into every configured target's `<commands>/tai/`
// subdirectory.
//
// The verb is intentionally a top-level command (not nested under
// `tai config` or `tai sync`) because it represents an explicit user
// gesture distinct from "load the source repo" (`tai sync`) — TAI's
// own integration assets opt-in rather than ride along with every
// sync. See specs/install-commands/spec.md for the contract.

package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/installcmd"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

func newInstallCommandsCommand() *cli.Command {
	return &cli.Command{
		Name:   "install-commands",
		Usage:  "Install TAI's bundled built-in slash commands into every configured target",
		Action: runInstallCommands,
	}
}

func runInstallCommands(_ context.Context, c *cli.Command) error {
	path, err := config.ResolvePath()
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, err.Error())
	}
	f, err := config.Load(path)
	if err != nil {
		return err
	}

	res, err := installcmd.Install(f, c.ErrWriter)
	if err != nil {
		return err
	}

	// One-line stdout summary so pipelines see the result. Stderr
	// already carries any falsy-skip warnings the installer emitted.
	//
	// All-skipped case: when every configured target's `commands`
	// sub-path is falsy, res.Targets is 0 and res.Skipped > 0.
	// Reporting "installed 0 commands into 0 targets" reads as a
	// successful no-op and hides the skip; emit a distinct line
	// instead.
	switch {
	case res.Targets == 0 && res.Skipped > 0:
		_, _ = fmt.Fprintf(c.Writer,
			"all %d target%s skipped — nothing installed\n",
			res.Skipped, plural(res.Skipped))
	case res.Removed > 0:
		_, _ = fmt.Fprintf(c.Writer,
			"installed %d command%s into %d target%s (%d stale built-in%s removed)\n",
			res.Written, plural(res.Written),
			res.Targets, plural(res.Targets),
			res.Removed, plural(res.Removed))
	default:
		_, _ = fmt.Fprintf(c.Writer,
			"installed %d command%s into %d target%s\n",
			res.Written, plural(res.Written),
			res.Targets, plural(res.Targets))
	}
	return nil
}

// plural returns "s" when n != 1, "" otherwise. Avoids the pkg-level
// equivalent in core/internal/sync to keep cmd self-contained.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
