package cmd

import (
	"context"
	"fmt"
	"io"
	"slices"
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
	if board.IsServeChild() {
		return runBoardServer(ctx, c)
	}
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
	scopeFlagNames := []string{"--" + prFlag, "--" + branchFlag}
	for _, a := range args {
		name := strings.SplitN(a, "=", 2)[0]
		if slices.Contains(scopeFlagNames, name) {
			return errcode.Newf(errcode.UnknownSubcommand,
				"%s is not accepted by `tai triage board`", name).
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

	b, body, err := readBriefing(c.Reader)
	if err != nil {
		return err
	}

	return board.Launch(body, func(url string) {
		_, _ = fmt.Fprintf(c.Writer, "Board ready at %s\n", url)
		_, _ = fmt.Fprintf(c.Writer, "Decide what you can, then submit. Anything you leave alone goes to the conversation.\n")
		_, _ = fmt.Fprintf(c.Writer, "Read the decisions with `tai triage board intents %s`.\n",
			scopeFlagsOf(b.Scope))
	})
}

// runBoardServer is the board's server process. It is this same binary
// re-executed with ServeChildEnv set, so the argument grammar and the
// briefing on stdin are identical to the launching command's — the only
// difference is that its stdout is a pipe back to that command rather
// than a terminal, and it serves rather than returning.
//
// The briefing is validated a second time here rather than trusted from
// the pipe. It costs microseconds, and the alternative is a server
// process whose behaviour depends on a contract nothing checks.
func runBoardServer(ctx context.Context, c *cli.Command) error {
	b, _, err := readBriefing(c.Reader)
	if err != nil {
		return err
	}

	entries, err := board.ServeDetached(ctx, b, c.Writer)
	if err != nil {
		return err
	}

	if _, err := board.WriteIntents(board.NewIntents(b.Repo, b.Scope, entries)); err != nil {
		return err
	}
	return nil
}

// readBriefing reads, decodes and validates stdin, returning the decoded
// briefing alongside the bytes it came from. No process is started and
// nothing is bound until it returns cleanly: a rejected briefing must
// leave no trace.
//
// The raw bytes are returned because the server process is fed the same
// briefing this one validated. Re-encoding the decoded value would send
// it something subtly different from what passed the schema check.
func readBriefing(r io.Reader) (board.Briefing, []byte, error) {
	limited := io.LimitReader(r, maxBriefingStdinBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return board.Briefing{}, nil, errcode.Wrap(errcode.TriageBoardInvalidJSON, err,
			"reading the briefing from stdin")
	}
	if int64(len(body)) > maxBriefingStdinBytes {
		return board.Briefing{}, nil, errcode.Newf(errcode.TriageBoardInvalidJSON,
			"briefing exceeds the %d-byte stdin limit", maxBriefingStdinBytes).
			WithHelp("brief one scope at a time")
	}

	b, decodeErr := board.DecodeBytes(body)
	if decodeErr != nil {
		return board.Briefing{}, nil, errcode.Wrap(errcode.TriageBoardInvalidJSON, decodeErr,
			"decoding the briefing").
			WithHelp("verify the briefing is valid JSON and matches the schema in the /tai-triage:triage command")
	}

	if vErrs := board.Validate(b); len(vErrs) > 0 {
		return board.Briefing{}, nil, briefingInvalidError(vErrs)
	}
	return b, body, nil
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

// scopeFlagsOf renders the scope as the flags `tai triage board intents`
// takes, so the launching command can hand the developer the exact line
// that reads their decisions back.
func scopeFlagsOf(s board.Scope) string {
	if s.Kind == "branch" {
		return "--branch " + s.Branch
	}
	if s.PR != nil {
		return fmt.Sprintf("--pr %d", *s.PR)
	}
	return "--pr <number>"
}
