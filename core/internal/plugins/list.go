package plugins

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
)

// List prints the installed-plugins table to w. When the state has
// zero entries, prints the literal `(no plugins installed)\n` so the
// output is greppable. Returns an error only if writes to w fail.
func List(state *State, w io.Writer) error {
	if state == nil || len(state.Plugins) == 0 {
		_, err := io.WriteString(w, "(no plugins installed)\n")
		return err
	}

	// Stable sort by name so two runs against the same state file
	// emit identical bytes (deterministic output for AI agents).
	rows := make([]Entry, len(state.Plugins))
	copy(rows, state.Plugins)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "name\tversion\tinstalled-at"); err != nil {
		return err
	}
	for _, e := range rows {
		ts := e.InstalledAt.UTC().Format("2006-01-02T15:04:05Z")
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Name, e.Version, ts); err != nil {
			return err
		}
	}
	return tw.Flush()
}
