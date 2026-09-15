package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/board"
	"github.com/urfave/cli/v3"
)

// newBoardIntentsCommand wires `tai triage board intents`.
//
// Unlike the board itself this verb resolves scope the way every other
// triage verb does, so the AI can ask for a scope's intents with the
// same flags it uses everywhere else.
func newBoardIntentsCommand() *cli.Command {
	return &cli.Command{
		Name:  "intents",
		Usage: "Emit the intents a submitted board recorded for a scope",
		Flags: scopeFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			return runBoardIntents(ctx, c)
		},
	}
}

func runBoardIntents(ctx context.Context, c *cli.Command) error {
	s, db, err := openDBAndScope(ctx, c)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	bs := board.Scope{Kind: string(s.Kind)}
	if s.Kind == "pr" {
		n := s.PRNumber
		bs.PR = &n
	} else {
		bs.Branch = s.BranchName
	}

	in, found, err := board.ReadIntents(s.OwnerName, bs)
	if err != nil {
		return err
	}
	if !found {
		return errcode.Newf(errcode.TriageNoIntents,
			"no board intents recorded for %s %s", s.OwnerName, s.TargetLabel()).
			WithHelp(
				"run the board first: investigate the pending comments, then pipe the briefing to `tai triage board -`",
				"if a board is open for this scope, it has not been submitted yet — the artifact is written on submit",
			)
	}

	_, _ = io.WriteString(c.Writer, formatIntents(in, s.OwnerName, s.TargetLabel()))
	return nil
}

// formatIntents renders the artifact for the AI: the submit time, then
// one line per entry in the order the board presented them.
func formatIntents(in board.Intents, repo, label string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Board intents — %s %s\n\n", repo, label)
	fmt.Fprintf(&b, "Submitted at %s\n\n", in.SubmittedAt)
	if len(in.Entries) == 0 {
		b.WriteString("(no comments were briefed)\n")
		return b.String()
	}
	for _, e := range in.Entries {
		fmt.Fprintf(&b, "- %d: %s", e.ID, e.Intent)
		if e.Note != "" {
			fmt.Fprintf(&b, " — %s", e.Note)
		}
		b.WriteString("\n")
	}
	return b.String()
}
