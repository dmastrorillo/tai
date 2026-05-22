package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/dmastrorillo/tai/plugins/triage/internal/triage"
	"github.com/dmastrorillo/tai/plugins/triage/internal/triage/scope"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

func newListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List review comments in the resolved scope as a table",
		Flags: append(scopeFlags(),
			&cli.StringSliceFlag{Name: statusFlag, Usage: "Filter by status (repeatable)"},
		),
		Action: func(ctx context.Context, c *cli.Command) error {
			return runList(ctx, c)
		},
	}
}

func runList(ctx context.Context, c *cli.Command) error {
	statuses := c.StringSlice(statusFlag)
	if err := triage.ValidateStatuses(statuses); err != nil {
		return err
	}

	s, db, err := openDBAndScope(ctx, c)
	if err != nil {
		return err
	}
	defer db.Close()

	comments, err := triage.ListComments(ctx, db, s, statuses)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Repo: %s   Scope: %s\n\n", s.OwnerName, listScopeLabel(s))
	_, _ = io.WriteString(c.Writer, header)

	if len(comments) == 0 {
		_, _ = io.WriteString(c.Writer, "(no comments)\n")
		return nil
	}

	width := detectWidth(c.Writer)
	_, _ = io.WriteString(c.Writer, "  ID  SEV    STATUS     BATCH  FILE                      TITLE\n")
	for _, cmt := range comments {
		batchKey := "-"
		if cmt.BatchKey.Valid {
			batchKey = cmt.BatchKey.String
		}
		fileLines := fmt.Sprintf("%s:%s", cmt.File, cmt.Lines)
		row := fmt.Sprintf("  %-3d %-6s %-10s %-6s %-25s ",
			cmt.Position, abbrevSeverity(cmt.Severity), cmt.Status, batchKey, truncate(fileLines, 25))
		title := truncate(cmt.Title, width-len(row))
		_, _ = fmt.Fprintf(c.Writer, "%s%s\n", row, title)
	}
	return nil
}

// listScopeLabel renders the scope label used in the list/show header
// (richer than scope.TargetLabel — PR scope includes the title).
func listScopeLabel(s scope.Scope) string {
	if s.Kind == scope.KindPR {
		if s.Title != "" {
			return fmt.Sprintf("PR #%d — %s", s.PRNumber, s.Title)
		}
		return fmt.Sprintf("PR #%d", s.PRNumber)
	}
	return "branch " + s.BranchName
}

func abbrevSeverity(sev string) string {
	switch sev {
	case "critical":
		return "crit"
	case "major":
		return "maj"
	case "minor":
		return "min"
	case "nitpick":
		return "nit"
	}
	return sev
}

// truncate clips s to at most max runes, replacing the last rune with
// `…` when truncation is needed. Operates on runes (not bytes) so a
// non-ASCII file path isn't split mid-character.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// detectWidth returns the terminal width when writer is a TTY, or 80
// when it isn't (the fallback the spec requires).
func detectWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 80
	}
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		return 80
	}
	width, _, err := term.GetSize(fd)
	if err != nil || width <= 0 {
		return 80
	}
	return width
}
