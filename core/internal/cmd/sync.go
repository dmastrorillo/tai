// `tai sync` verb — the surface that wires core/internal/sync to the
// CLI. Action stays small; Sync orchestration lives in the package.

package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/datadir"
)

func newSyncCommand() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Pull the configured source repo and copy its assets into every configured target",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip the overwrite/prune confirmation prompt",
			},
			&cli.BoolFlag{
				Name:  "prune",
				Usage: "Delete files no longer present in the source (use with care)",
			},
		},
		Action: runSync,
	}
}

func runSync(ctx context.Context, c *cli.Command) error {
	cfgPath, err := resolveConfigPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	dataDir, err := datadir.EnsureWritable()
	if err != nil {
		return err
	}

	res, err := sync.Sync(ctx, cfg, dataDir, sync.Options{
		Yes:    c.Bool("yes"),
		Prune:  c.Bool("prune"),
		Stdin:  c.Reader,
		Stdout: c.Writer,
		Stderr: c.ErrWriter,
	})
	if err != nil {
		return err
	}

	// Summary line on stderr. Cancelled syncs have their own
	// cancellation notice already; don't double-print.
	if !res.Cancelled {
		_, _ = io.WriteString(c.ErrWriter, summariseResult(res))
	}
	return nil
}

func summariseResult(r *sync.Result) string {
	switch {
	case r.Written == 0 && r.Pruned == 0:
		return "[tai] sync: no changes\n"
	default:
		return fmt.Sprintf("[tai] sync: wrote %d, overwrote %d, pruned %d\n",
			r.Written, r.Overwritten, r.Pruned)
	}
}
