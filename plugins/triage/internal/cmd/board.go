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

// maxBriefingStdinBytes caps the briefing so a misdirected pipe cannot
// make io.ReadAll allocate without bound. A briefing carries seven prose
// fields per comment; even a large review stays far under this.
const maxBriefingStdinBytes = 8 << 20 // 8 MB

// newBoardCommand wires `tai triage board -` and its `intents` child.
//
// The board is the bulk-decision surface. The AI investigates every
// pending comment — the same investigation the triage loop performs
// before presenting one — and pipes the result here as a briefing. The
// developer decides in one pass; the board records intents and the
// conversation persists them.
//
// The board holds no database handle and shells out to nothing. Two of
// the seven fields it renders are not stored: `cause` is always derived
// from reading the code, and `why_fix` and `suggested_fix` are worked
// out when the record lacks them. There is nothing in the database for
// it to read.
//
// It therefore takes no --pr / --branch flags either: the shared
// scope-resolution rule reads the current git branch and the prs /
// branches tables, and the briefing already carries repo and scope.
func newBoardCommand() *cli.Command {
	return &cli.Command{
		Name:  "board",
		Usage: "Serve a browser board from a briefing on stdin for bulk decisions",
		Commands: []*cli.Command{
			newBoardIntentsCommand(),
		},
		// The board's argument grammar is exactly one positional `-`.
		// With flag parsing on, urfave/cli silently discards anything
		// that follows a positional, so `board - --pr 142` would serve
		// a scope the caller believes they overrode. Taking the
		// arguments raw is the only way to see — and refuse — them.
		SkipFlagParsing: true,
		Action: func(ctx context.Context, c *cli.Command) error {
			return runBoard(ctx, c)
		},
	}
}

func runBoard(ctx context.Context, c *cli.Command) error {
	if c.IsSet(RepoFlag) {
		return errcode.New(errcode.UnknownSubcommand,
			"--repo is not accepted by `tai triage board` (repo identity is read from the briefing)").
			WithHelp("remove --repo and let the briefing's `repo` field name the scope")
	}

	args := c.Args().Slice()
	if len(args) == 0 {
		return errcode.New(errcode.UnknownSubcommand,
			"tai triage board expects '-' to read the briefing from stdin").
			WithHelp("invoke as `tai triage board -` and pipe the briefing on stdin")
	}
	for _, a := range args {
		if a == "--pr" || a == "--branch" || strings.HasPrefix(a, "--pr=") || strings.HasPrefix(a, "--branch=") {
			return errcode.Newf(errcode.UnknownSubcommand,
				"%s is not accepted by `tai triage board`", strings.SplitN(a, "=", 2)[0]).
				WithHelp(
					"the board takes its scope from the briefing's `repo` and `scope` fields",
					"the shared scope-resolution rule reads the current git branch and the database, and the board does neither",
				)
		}
	}
	if len(args) > 1 || args[0] != "-" {
		return errcode.Newf(errcode.UnknownSubcommand,
			"tai triage board expects '-' as its sole positional argument, got %q", args[0]).
			WithHelp(
				"invoke as `tai triage board -` and pipe the briefing on stdin",
				"the scope comes from the briefing's `repo` and `scope` fields, not from flags",
			)
	}

	b, err := readBriefing(c.Reader)
	if err != nil {
		return err
	}

	srv, err := board.NewServer(b)
	if err != nil {
		return err
	}

	entries, err := srv.Serve(ctx, func(url string) {
		_, _ = fmt.Fprintf(c.Writer, "Board ready at %s\n", url)
		_, _ = fmt.Fprintf(c.Writer, "Decide what you can, then submit. Anything you leave alone goes to the conversation.\n")
	})
	if err != nil {
		return err
	}

	path, err := board.WriteIntents(board.NewIntents(b.Repo, b.Scope, entries))
	if err != nil {
		return err
	}
	noun := "intents"
	if len(entries) == 1 {
		noun = "intent"
	}
	_, _ = fmt.Fprintf(c.Writer, "Recorded %d %s for %s %s.\n",
		len(entries), noun, b.Repo, scopeLabelOf(b.Scope))
	_, _ = fmt.Fprintf(c.Writer, "  %s\n", path)
	return nil
}

// readBriefing reads, decodes and validates stdin. Nothing is bound and
// no browser is launched until it returns cleanly: a rejected briefing
// must leave no trace.
func readBriefing(r io.Reader) (board.Briefing, error) {
	limited := io.LimitReader(r, maxBriefingStdinBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return board.Briefing{}, errcode.Wrap(errcode.TriageBoardInvalidJSON, err,
			"reading the briefing from stdin")
	}
	if int64(len(body)) > maxBriefingStdinBytes {
		return board.Briefing{}, errcode.Newf(errcode.TriageBoardInvalidJSON,
			"briefing exceeds the %d-byte stdin limit", maxBriefingStdinBytes).
			WithHelp("brief one scope at a time")
	}

	b, decodeErr := board.DecodeBytes(body)
	if decodeErr != nil {
		return board.Briefing{}, errcode.Wrap(errcode.TriageBoardInvalidJSON, decodeErr,
			"decoding the briefing").
			WithHelp("verify the briefing is valid JSON and matches the schema in the /tai-triage:triage command")
	}

	if vErrs := board.Validate(b); len(vErrs) > 0 {
		return board.Briefing{}, briefingInvalidError(vErrs)
	}
	return b, nil
}

// briefingInvalidError renders every violation as its own Help bullet.
// Reporting one at a time would cost the AI assembling the briefing a
// round trip for each.
func briefingInvalidError(vErrs []board.ValidationError) error {
	sorted := board.SortErrors(vErrs)
	noun := "problems"
	if len(sorted) == 1 {
		noun = "problem"
	}
	e := errcode.Newf(errcode.TriageBoardSchemaInvalid,
		"%d %s with the briefing", len(sorted), noun)

	bullets := make([]string, 0, len(sorted)+1)
	for _, ve := range sorted {
		bullets = append(bullets, ve.Path+": "+ve.Message)
	}
	bullets = append(bullets,
		"fix every path listed above and pipe the briefing again — they are all reported together so one regeneration is enough")
	return e.WithHelp(bullets...)
}

func scopeLabelOf(s board.Scope) string {
	if s.Kind == "branch" {
		return "branch " + s.Branch
	}
	if s.PR != nil {
		return fmt.Sprintf("PR #%d", *s.PR)
	}
	return "PR"
}

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
