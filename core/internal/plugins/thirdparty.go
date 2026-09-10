// Third-party plugin consent.
//
// A plugin is a binary tai downloads and executes. For a built-in
// registry entry that binary is tai's own release; for anything else
// it is a stranger's code running with the user's privileges. The gate
// below is the one place that distinction is enforced, so install and
// update cannot drift apart on it.
//
// Spec: openspec/specs/plugin-host/spec.md
// §"Third-party plugin install confirmation".

package plugins

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// IsBuiltin reports whether src points at a first-party plugin — an
// exact host/repo/subpath match against a built-in registry entry.
//
// Version is deliberately not compared: pinning a tag with
// `--version` does not turn a first-party plugin into someone else's
// code.
func IsBuiltin(src Source) bool {
	for _, entry := range builtin {
		if entry.Host == src.Host && entry.Repo == src.Repo && entry.Subpath == src.Subpath {
			return true
		}
	}
	return false
}

// confirmThirdParty returns nil when name may be fetched from src, and
// a PLUGIN_THIRDPARTY_UNCONFIRMED error when the user has not agreed.
//
// interactive says whether there is a person on the other end of
// stdin. Without one, an unanswered prompt would hang a script
// forever, so the refusal is immediate and names the flag that
// unblocks it.
func confirmThirdParty(name string, src Source, stdin io.Reader, stderr io.Writer, assumeYes, interactive bool) error {
	if IsBuiltin(src) || assumeYes {
		return nil
	}
	if !interactive {
		return unconfirmedError(name, src)
	}

	_, _ = fmt.Fprintf(stderr,
		"Installing third-party plugin %s from %s. Third-party plugins run arbitrary code on your machine. Continue? [y/N] ",
		name, sourceLabel(src))

	line, _ := bufio.NewReader(stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	return unconfirmedError(name, src)
}

func unconfirmedError(name string, src Source) error {
	return errcode.Newf(errcode.PluginThirdpartyUnconfirmed,
		"installing %q from %s needs confirmation: it is not a built-in plugin",
		name, sourceLabel(src)).
		WithHelp(
			"re-run with `--yes` to confirm you trust this source",
			"or run the command in a terminal to be asked interactively",
			"built-in plugins install without confirmation; `tai plugins list` shows what you already have",
		)
}

// sourceLabel renders a Source the way the user typed it into
// `--source`, so the prompt names something they recognise.
func sourceLabel(src Source) string {
	label := src.Host + "/" + src.Repo
	if src.Subpath != "" {
		label += "/" + src.Subpath
	}
	return label
}
