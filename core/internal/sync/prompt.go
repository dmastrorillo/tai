package sync

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Plan groups every category's would-create / would-overwrite paths
// across all configured targets, plus the orphan list. The prompt
// renders this batched view in a single user-visible block.
type Plan struct {
	// Overwrites groups paths that already exist at the destination
	// by category. Each entry is "<target-root>/<effective-subpath>/<rel>".
	Overwrites map[Category][]string
	// Orphans is the union of orphan entries (per category, but
	// represented as "<category>/<rel>" for table simplicity) across
	// all targets. Caller stuffs absolute paths into PrunePaths for
	// the actual delete walk.
	Orphans []string
	// PrunePaths carries the absolute filesystem paths the deletion
	// pass walks when --prune is on and the user confirms.
	PrunePaths []string
}

// HasOverwrites reports whether any category has a non-empty list.
func (p *Plan) HasOverwrites() bool {
	for _, paths := range p.Overwrites {
		if len(paths) > 0 {
			return true
		}
	}
	return false
}

// HasOrphans reports whether the plan would touch any orphan path.
// Distinct from "has orphans pending" because --prune is the only
// path that actually deletes; without it, orphans count but no
// confirmation is required.
func (p *Plan) HasOrphans() bool {
	return len(p.Orphans) > 0
}

// Prompt prints the batched plan to stderr and reads a y/N response
// from stdin. Returns true iff the user confirms.
//
// Sync calls this with confirmDelete=true ONLY when --prune is on and
// orphans are present; without --prune the orphans-pending summary
// goes through a separate non-blocking call (NoticeOrphans).
//
// When -y is set, callers MUST short-circuit BEFORE calling Prompt;
// this function unconditionally reads stdin.
func Prompt(p *Plan, confirmDelete bool, stdin io.Reader, stderr io.Writer) (bool, error) {
	var b strings.Builder
	b.WriteString("The following changes will be made:\n\n")

	for _, cat := range Categories() {
		paths := p.Overwrites[cat]
		if len(paths) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  Overwrite (%s):\n", cat)
		for _, p := range paths {
			fmt.Fprintf(&b, "    %s\n", p)
		}
	}

	if confirmDelete && len(p.Orphans) > 0 {
		b.WriteString("\n  Delete (orphans no longer in source):\n")
		for _, p := range p.Orphans {
			fmt.Fprintf(&b, "    %s\n", p)
		}
	}

	b.WriteString("\nProceed? [y/N] ")
	if _, err := io.WriteString(stderr, b.String()); err != nil {
		return false, err
	}

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		// EOF / no input → treat as N (the safe default).
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}

// NoticeOrphans writes the "N orphans pending — run `tai sync
// --prune`" summary to stderr. Called on every sync (per the spec)
// when orphans exist and --prune was NOT supplied.
func NoticeOrphans(stderr io.Writer, count int) {
	if count == 0 {
		return
	}
	_, _ = fmt.Fprintf(stderr, "[tai] %d orphan%s pending — run `tai sync --prune` to delete\n",
		count, plural(count))
}

// NoticeCancelled writes a one-line cancellation message to stderr
// for the "user answered N" path.
func NoticeCancelled(stderr io.Writer) {
	_, _ = io.WriteString(stderr, "Sync cancelled; no files written.\n")
}

// NoticeOverwritten writes (when -y is in play and overwrites
// happened) a brief stderr summary so the user can see what got
// touched.
func NoticeOverwritten(stderr io.Writer, overwrites map[Category][]string) {
	total := 0
	for _, paths := range overwrites {
		total += len(paths)
	}
	if total == 0 {
		return
	}
	_, _ = fmt.Fprintf(stderr, "[tai] overwrote %d file%s (-y bypassed the confirmation prompt)\n",
		total, plural(total))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
