package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/dmastrorillo/tai/pkg/errcode"
	importer "github.com/dmastrorillo/tai/plugins/triage/internal/import"
	"github.com/dmastrorillo/tai/plugins/triage/internal/import/payload"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
	"github.com/urfave/cli/v3"
)

// maxImportStdinBytes caps the size of an `tai import -` stdin payload.
// Payloads in practice are tens to low hundreds of KB; a misdirected
// pipe (e.g. `tai import - < /var/log/...`) would otherwise let
// io.ReadAll allocate without bound. Anything larger than this is
// vanishingly unlikely to be a real review payload.
const maxImportStdinBytes = 4 << 20 // 4 MB

// newImportCommand wires the `tai import` subcommand.
//
// `tai import` is the data-ingestion verb. It is unique within the CLI
// in two ways:
//
//   - It is repo-independent at the CLI boundary: the JSON payload
//     carries the repo identity in its `repo` field, so the global
//     `--repo` flag is not honoured (and is rejected with a usage
//     error when combined with this verb).
//   - It reads its input exclusively from stdin. The single positional
//     `-` is required as a documentation hint that "this command reads
//     stdin"; any other positional argument is a usage error.
//
// Errors surface via the foundation contract:
//
//   - IMPORT_INVALID_JSON  — stdin is not valid JSON (exit 1)
//   - IMPORT_SCHEMA_INVALID — JSON parses but fails schema rules (exit 3)
//   - IMPORT_AMBIGUOUS_REFS — external_refs resolve to multiple existing
//     comments (exit 3)
//   - UNKNOWN_SUBCOMMAND   — missing or wrong positional argument, or
//     `--repo` combined with this verb (exit 1)
//   - DB_CONSTRAINT_VIOLATION, DATA_DIR_UNWRITABLE, INTERNAL_ERROR —
//     propagated from the storage / importer layers.
func newImportCommand() *cli.Command {
	return &cli.Command{
		Name:  "import",
		Usage: "Read a validated JSON payload from stdin and persist it",
		Action: func(ctx context.Context, c *cli.Command) error {
			return runImport(ctx, c)
		},
	}
}

// runImport implements the `tai import -` action.
//
// Step 1: usage-shape validation. The repo flag is forbidden, exactly
// one positional `-` is required.
//
// Step 2: read stdin into memory (capped at maxImportStdinBytes).
// Buffering simplifies error reporting and lets the strict JSON
// decoder surface decode + trailing-content errors uniformly.
//
// Step 3: decode + validate the payload. Decode failures map to
// IMPORT_INVALID_JSON; validation failures map to IMPORT_SCHEMA_INVALID
// with each violation rendered as its own help bullet.
//
// Step 4: open the database, run the importer's upserts inside a single
// transaction, and write the success summary to stdout.
func runImport(ctx context.Context, c *cli.Command) error {
	if c.IsSet(RepoFlag) {
		return errcode.New(errcode.UnknownSubcommand,
			"--repo is not accepted by `tai import` (repo identity is read from the JSON payload)").
			WithHelp("remove --repo and let the JSON's `repo` field drive the import target")
	}

	args := c.Args().Slice()
	if len(args) == 0 {
		return errcode.New(errcode.UnknownSubcommand,
			"tai import expects '-' to read the JSON payload from stdin").
			WithHelp("invoke as `tai import -` and pipe the payload on stdin")
	}
	if len(args) > 1 || args[0] != "-" {
		return errcode.Newf(errcode.UnknownSubcommand,
			"tai import expects '-' as its sole positional argument, got %q", args[0]).
			WithHelp("invoke as `tai import -` and pipe the payload on stdin")
	}

	// io.LimitReader caps stdin so a runaway pipe can't OOM the process.
	// We read maxImportStdinBytes+1 so an exact-cap payload is still
	// accepted and only over-cap inputs fail.
	limited := io.LimitReader(c.Reader, maxImportStdinBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return errcode.Wrap(errcode.ImportInvalidJSON, err,
			"reading payload from stdin")
	}
	if int64(len(body)) > maxImportStdinBytes {
		return errcode.Newf(errcode.ImportInvalidJSON,
			"payload exceeds the %d-byte stdin limit", maxImportStdinBytes).
			WithHelp("split the review into smaller PR-scoped imports, or pipe a more focused payload")
	}

	p, decodeErr := payload.DecodeBytes(body)
	if decodeErr != nil {
		return errcode.Wrap(errcode.ImportInvalidJSON, decodeErr,
			"decoding JSON payload").
			WithHelp("verify the payload is valid JSON and matches the import schema")
	}

	if vErrs := payload.Validate(p); len(vErrs) > 0 {
		return schemaInvalidError(vErrs)
	}

	db, err := storage.Open(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	s, err := importer.Import(ctx, db, p)
	if err != nil {
		var ambig *importer.AmbiguousRefsError
		if errors.As(err, &ambig) {
			return errcode.Newf(errcode.ImportAmbiguousRefs,
				"comments[%d] external_refs resolve to multiple existing comments: %v",
				ambig.CommentIndex, ambig.CommentIDs).
				WithHelp(
					"remove one of the conflicting refs from the payload and re-run",
					"or use `tai forget` to delete the unwanted existing row, then re-run",
				)
		}
		return err
	}

	_, _ = io.WriteString(c.Writer, formatImportSummary(s))
	return nil
}

// schemaInvalidError renders a validation-error slice as an *errcode.Error
// with each violation as its own Help bullet. cliout.WriteError prints
// "What to do:" bullets one per line, which is the readable equivalent
// of the design doc's "Error: N problems …" multi-line block (the
// foundation contract collapses newlines inside the Error: line, so we
// route per-violation detail through Help instead).
func schemaInvalidError(vErrs []payload.ValidationError) error {
	sorted := append([]payload.ValidationError(nil), vErrs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})

	noun := "problems"
	if len(sorted) == 1 {
		noun = "problem"
	}
	e := errcode.Newf(errcode.ImportSchemaInvalid,
		"%d %s with the JSON payload", len(sorted), noun)

	bullets := make([]string, 0, len(sorted)+1)
	for _, ve := range sorted {
		bullets = append(bullets, ve.Path+": "+ve.Message)
	}
	bullets = append(bullets,
		"the /tai:import command emitted incomplete data — re-run /tai:import (or fix the JSON manually if you piped it from somewhere else)")
	return e.WithHelp(bullets...)
}

// formatImportSummary renders the success-output block per the spec.
// Lines whose counter is zero are suppressed; the header is always
// present and the closing `[exit 0]` tag terminates the block.
func formatImportSummary(s importer.Summary) string {
	out := fmt.Sprintf("Imported %s %s (%d comments, %d batches)\n",
		s.Repo, s.TargetLabel, s.CommentCount, s.BatchCount)
	if s.Inserted > 0 {
		out += fmt.Sprintf("  Inserted:  %d new comments\n", s.Inserted)
	}
	if s.Updated > 0 {
		out += fmt.Sprintf("  Updated:   %d existing comments (pending)\n", s.Updated)
	}
	if s.Frozen > 0 {
		out += fmt.Sprintf("  Frozen:    %d comments left untouched (already triaged)\n", s.Frozen)
	}
	if s.RefsAdded > 0 {
		out += fmt.Sprintf("  Refs added: %d external refs attached to existing comments\n", s.RefsAdded)
	}
	if s.BatchInserted > 0 || s.BatchUpdated > 0 {
		out += fmt.Sprintf("  Batches:   %d inserted, %d updated\n", s.BatchInserted, s.BatchUpdated)
	}
	out += "[exit 0]\n"
	return out
}
