package cmd

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dmastrorillo/tai/internal/errcode"
	"github.com/dmastrorillo/tai/internal/triage"
	"github.com/dmastrorillo/tai/internal/triage/scope"
	"github.com/urfave/cli/v3"
)

func newShowCommand() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: "Render one comment (or every comment with --all) as markdown",
		Flags: append(scopeFlags(),
			&cli.BoolFlag{Name: allFlag, Usage: "Render every comment in the resolved scope"},
			&cli.StringSliceFlag{Name: statusFlag, Usage: "Filter by status (only valid with --all)"},
		),
		Action: func(ctx context.Context, c *cli.Command) error {
			return runShow(ctx, c)
		},
	}
}

func runShow(ctx context.Context, c *cli.Command) error {
	all := c.Bool(allFlag)
	statuses := c.StringSlice(statusFlag)
	args := c.Args().Slice()

	if !all {
		if len(args) == 0 {
			return errcode.New(errcode.TriageInvalidFlags,
				"tai show requires a comment id, or --all").
				WithHelp("invoke as `tai show <id>` or `tai show --all`")
		}
		if len(args) > 1 {
			return errcode.Newf(errcode.TriageInvalidFlags,
				"tai show takes exactly one comment id, got %d", len(args)).
				WithHelp("pass a single position, e.g. `tai show 1`")
		}
		if len(statuses) > 0 {
			return errcode.New(errcode.TriageInvalidFlags,
				"--status is only valid with --all").
				WithHelp("drop --status, or switch to `tai show --all`")
		}
	} else if len(args) > 0 {
		return errcode.New(errcode.TriageInvalidFlags,
			"tai show --all takes no positional arguments").
			WithHelp("invoke as `tai show --all` (optionally with --status)")
	}

	if err := triage.ValidateStatuses(statuses); err != nil {
		return err
	}

	s, db, err := openDBAndScope(ctx, c)
	if err != nil {
		return err
	}
	defer db.Close()

	if all {
		comments, err := triage.ListComments(ctx, db, s, statuses)
		if err != nil {
			return err
		}
		writeAllBlocks(c.Writer, s, comments)
		return nil
	}

	pos, err := strconv.Atoi(args[0])
	if err != nil {
		return errcode.Newf(errcode.TriageInvalidFlags,
			"comment id %q is not a positive integer", args[0]).
			WithHelp("pass a position like 1, 2, ...")
	}
	cmt, total, err := triage.Show(ctx, db, s, pos)
	if err != nil {
		return err
	}
	writeBlock(c.Writer, s, cmt, total)
	return nil
}

// writeAllBlocks emits one block per comment, joined by the spec's
// `\n\n---\n\n` separator (blank line, `---`, blank line).
//
// Each block ends with a trailing `\n` from its last `section()`
// call. The separator below contributes the additional `\n` that
// produces the leading blank line, the `---` rule, and the trailing
// blank line that precedes the next block's `# ...` header. If a
// future change drops the trailing newline from the last section,
// fix this separator to `"\n\n---\n\n"` to keep the contract.
func writeAllBlocks(w io.Writer, s scope.Scope, cs []triage.Comment) {
	total := len(cs)
	for i, c := range cs {
		if i > 0 {
			_, _ = io.WriteString(w, "\n---\n\n")
		}
		writeBlock(w, s, c, total)
	}
}

// writeBlock emits one comment as the spec's stable markdown shape.
func writeBlock(w io.Writer, s scope.Scope, c triage.Comment, total int) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s — comment %d of %d\n\n", s.OwnerName, listScopeLabel(s), c.Position, total)

	fmt.Fprintf(&b, "**Severity:** %s  **Category:** %s  **Status:** %s\n",
		c.Severity, c.Category, c.Status)
	fmt.Fprintf(&b, "**File:** `%s:%s`\n", c.File, c.Lines)
	fmt.Fprintf(&b, "**Source:** %s\n", c.Source)
	if c.BatchKey.Valid {
		title := ""
		if c.BatchTitle.Valid {
			title = c.BatchTitle.String
		}
		fmt.Fprintf(&b, "**Batch:** %s — %s\n", c.BatchKey.String, title)
	}

	section(&b, "Title", c.Title)
	section(&b, "Description", c.Description)
	section(&b, "Why fix it", c.WhyFix)
	section(&b, "Suggested fix", c.SuggestedFix)
	section(&b, "What happens if you don't fix it", c.Consequences)

	if (c.Status == "accepted" || c.Status == "completed") && c.Resolution.Valid && c.Resolution.String != "" {
		section(&b, "Resolution", c.Resolution.String)
	}
	if c.Status == "dismissed" {
		reason := ""
		if c.DismissReason.Valid {
			reason = c.DismissReason.String
		}
		by := ""
		if c.DismissedBy.Valid {
			by = c.DismissedBy.String
		}
		section(&b, "Dismissed because", fmt.Sprintf("%s (by %s)", reason, by))
	}

	_, _ = io.WriteString(w, b.String())
}

func section(b *strings.Builder, heading, body string) {
	fmt.Fprintf(b, "\n## %s\n%s\n", heading, body)
}
