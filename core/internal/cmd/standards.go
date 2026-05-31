// `tai standards` verb tree: the AI-facing surface for the source
// repo's standards library.
//
// `tai standards list` enumerates every loadable standard;
// `tai standards load <name>` prints the markdown body (with
// frontmatter stripped) so the AI session can ingest the standard
// without parsing the file itself. Invoking the bare verb
// (`tai standards` with no subcommand) shows subcommand help.

package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/standards"
	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/datadir"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

func newStandardsCommand() *cli.Command {
	return &cli.Command{
		Name:  "standards",
		Usage: "List or load standards defined in the source repo",

		Commands: []*cli.Command{
			newStandardsListCommand(),
			newStandardsLoadCommand(),
		},

		Action: func(_ context.Context, c *cli.Command) error {
			return cli.ShowSubcommandHelp(c)
		},
	}
}

func newStandardsListCommand() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "List every standard available in the source repo",
		Action: runStandardsList,
	}
}

func runStandardsList(_ context.Context, c *cli.Command) error {
	stds, err := loadStandards(c.ErrWriter)
	if err != nil {
		return err
	}
	if len(stds) == 0 {
		_, _ = io.WriteString(c.Writer, "(no standards)\n")
		return nil
	}

	var b strings.Builder
	for _, s := range stds {
		fmt.Fprintf(&b, "%s  %s\n", s.Name, s.Description)
	}
	_, _ = io.WriteString(c.Writer, b.String())
	return nil
}

func newStandardsLoadCommand() *cli.Command {
	return &cli.Command{
		Name:      "load",
		Usage:     "Print the body of the named standard (frontmatter stripped)",
		ArgsUsage: "<name>",
		Action:    runStandardsLoad,
	}
}

func runStandardsLoad(_ context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) != 1 {
		return errcode.New(errcode.MissingArg,
			"tai standards load requires exactly one argument: <name>").
			WithHelp("example: `tai standards load sdlc`")
	}
	name := args[0]

	stds, err := loadStandards(c.ErrWriter)
	if err != nil {
		return err
	}
	s, ok := standards.Find(stds, name)
	if !ok {
		return errcode.Newf(errcode.StandardNotFound,
			"no standard named %q", name).
			WithHelp(
				"run `tai standards list` to see available standards",
				"if the source repo has changed, run `tai sync` first",
			)
	}

	_, _ = c.Writer.Write(s.Body)
	return nil
}

// loadStandards resolves the data directory and invokes the standards
// loader against the clone path. Mirrors loadWorkflows in workflow.go;
// the two are split rather than abstracted to keep each verb's wiring
// readable on its own.
func loadStandards(errSink io.Writer) ([]standards.Standard, error) {
	dataDir, err := datadir.Resolve()
	if err != nil {
		return nil, err
	}
	cloneDir := sync.CloneDir(dataDir)
	return standards.Load(cloneDir, errSink)
}
