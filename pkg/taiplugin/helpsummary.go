package taiplugin

import (
	"io"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// HelpSummaryFlag is the wire-contract argument the host passes to a
// plugin binary to ask what the plugin is for. Part of the append-only
// wire contract: the spelling SHALL NOT change without a major version
// bump of tai.
const HelpSummaryFlag = "--help-summary"

// HelpSummary implements the `<plugin> --help-summary` wire verb.
//
// The host invokes it during install and update, reads the first line
// of stdout, and stores it as the plugin's `description` in
// <TAI_DATA_DIR>/state/plugins.json — the text `tai --help` lists
// beside the plugin's name. A plugin that does not answer cannot be
// installed (the host fails with PLUGIN_HELP_SUMMARY_FAILED), so
// every SDK-built plugin gets the verb from this one call.
//
// Call it in main before running the command tree, and exit
// successfully when it reports handled:
//
//	if handled, err := taiplugin.HelpSummary(os.Stdout, os.Args, "Does the thing."); handled {
//		os.Exit(cliexec.Exit(os.Stderr, err))
//	}
//
// description should be a single line of 80 characters or fewer
// stating what the plugin does. Surrounding whitespace is trimmed and
// only the first line is written, each followed by exactly one "\n".
// An empty description is an author error and returns
// PLUGIN_HELP_SUMMARY_FAILED with nothing written, so the mistake
// surfaces when the author runs their own plugin rather than at
// install time on a user's machine.
//
// handled is true whenever the flag appears in args, including on the
// error path — the caller must not fall through to its command tree,
// which would print unrelated output on the host's stdout.
func HelpSummary(w io.Writer, args []string, description string) (handled bool, err error) {
	if !requestsHelpSummary(args) {
		return false, nil
	}

	line, _, _ := strings.Cut(strings.TrimSpace(description), "\n")
	line = strings.TrimSpace(line)
	if line == "" {
		return true, errcode.New(errcode.PluginHelpSummaryFailed,
			"plugin help summary is empty").
			WithHelp(
				"pass a one-line description of at most 80 characters to taiplugin.HelpSummary",
				"the host stores it as the plugin's description and shows it in `tai --help`",
			)
	}

	if _, werr := io.WriteString(w, line+"\n"); werr != nil {
		return true, errcode.Wrap(errcode.PluginHelpSummaryFailed, werr,
			"write plugin help summary")
	}
	return true, nil
}

// requestsHelpSummary reports whether args (an os.Args-shaped slice,
// program name first) carries the wire flag. Matched exactly: a verb
// or value that merely resembles the flag does not count.
func requestsHelpSummary(args []string) bool {
	for _, a := range args[min(1, len(args)):] {
		if a == HelpSummaryFlag {
			return true
		}
	}
	return false
}
