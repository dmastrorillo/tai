// `tai workflow` verb tree: the AI-facing surface for the source
// repo's workflow library.
//
// `tai workflow list` enumerates every loadable workflow; `tai
// workflow run <name>` emits a markdown plan an AI session consumes
// to execute the workflow. The plan is descriptive, not prescriptive
// — TAI does not invoke skills/commands itself. Invoking the bare
// verb (`tai workflow` with no subcommand) shows subcommand help.
//
// The clone is at <TAI_DATA_DIR>/source/; if the user has not yet
// run `tai sync`, the workflows tree is absent and both verbs fall
// back to the "no workflows" path.

package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/core/internal/workflow"
	"github.com/dmastrorillo/tai/pkg/datadir"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

func newWorkflowCommand() *cli.Command {
	return &cli.Command{
		Name:  "workflow",
		Usage: "List or render workflows defined in the source repo",

		Commands: []*cli.Command{
			newWorkflowListCommand(),
			newWorkflowRunCommand(),
		},

		Action: func(_ context.Context, c *cli.Command) error {
			return cli.ShowSubcommandHelp(c)
		},
	}
}

func newWorkflowListCommand() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "List every workflow available in the source repo",
		Action: runWorkflowList,
	}
}

func runWorkflowList(_ context.Context, c *cli.Command) error {
	workflows, err := loadWorkflows(c.ErrWriter)
	if err != nil {
		return err
	}
	if len(workflows) == 0 {
		_, _ = io.WriteString(c.Writer, "(no workflows)\n")
		return nil
	}

	// Format: "<colon-name>  <description>" — two spaces between
	// columns matches the spec's worked examples and is what the
	// standards-list verb also uses. workflow.Load guarantees
	// Description is non-empty (parseWorkflowFile rejects missing
	// descriptions with WORKFLOW_INVALID), so no fallback string is
	// needed here.
	var b strings.Builder
	for _, wf := range workflows {
		fmt.Fprintf(&b, "%s  %s\n", wf.Name, wf.Description)
	}
	_, _ = io.WriteString(c.Writer, b.String())
	return nil
}

func newWorkflowRunCommand() *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "Print the markdown plan an AI session should follow to execute <name>",
		ArgsUsage: "<name>",
		Action:    runWorkflowRun,
	}
}

func runWorkflowRun(_ context.Context, c *cli.Command) error {
	name, err := requireOneArg(c, "tai workflow run", "<name>",
		"example: `tai workflow run propose`")
	if err != nil {
		return err
	}

	workflows, err := loadWorkflows(c.ErrWriter)
	if err != nil {
		return err
	}
	wf, ok := workflow.Find(workflows, name)
	if !ok {
		return errcode.Newf(errcode.WorkflowNotFound,
			"no workflow named %q", name).
			WithHelp(
				"run `tai workflow list` to see available workflows",
				"if the source repo has changed, run `tai sync` first",
			)
	}

	_, _ = io.WriteString(c.Writer, renderWorkflowPlan(wf))
	return nil
}

// loadWorkflows resolves the data directory, derives the clone path,
// and invokes the workflow loader. Warnings from the loader (e.g.
// case-insensitive name collisions) go to errSink so the user sees
// them on stderr without polluting the data channel.
func loadWorkflows(errSink io.Writer) ([]workflow.Workflow, error) {
	dataDir, err := datadir.Resolve()
	if err != nil {
		return nil, err
	}
	cloneDir := sync.CloneDir(dataDir)
	return workflow.Load(cloneDir, errSink)
}

// renderWorkflowPlan emits the markdown plan TC-WF-011 pins. The
// layout is the spec's contract — H1, description paragraph,
// Required tools, Steps, Failure mode — and intentionally is the only
// shape any AI agent has to parse.
func renderWorkflowPlan(wf workflow.Workflow) string {
	// kind widths: today {"skill" (5), "command" (7)}. Pad to the
	// longest kind so the bullet column lines up; recompute per
	// workflow in case future kinds change the max.
	kindWidth := 0
	for _, s := range wf.Steps {
		if len(s.Kind) > kindWidth {
			kindWidth = len(s.Kind)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Workflow: %s\n\n", wf.Name)
	fmt.Fprintf(&b, "%s\n\n", wf.Description)

	b.WriteString("## Required tools\n\n")
	for _, s := range wf.Steps {
		// "<kind>:" left-justified to kindWidth+1 (the colon), two
		// spaces, then `/<name>`.
		label := s.Kind + ":"
		fmt.Fprintf(&b, "- %-*s  /%s\n", kindWidth+1, label, s.Name)
	}
	b.WriteString("\n")

	b.WriteString("## Steps\n\n")
	for i, s := range wf.Steps {
		fmt.Fprintf(&b, "%d. Invoke `/%s` (%s).\n", i+1, s.Name, s.Kind)
	}
	b.WriteString("\n")

	b.WriteString("## Failure mode\n\n")
	b.WriteString("If any required tool listed above is unavailable in your session, ")
	b.WriteString("report exactly which tools are missing and abort the workflow. ")
	b.WriteString("Do not substitute alternatives or proceed with a partial set.\n")

	return b.String()
}
