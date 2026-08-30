package plugins

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/pkg/taiplugin"
)

// maxHelpSummaryBytes caps what the host will read from a plugin's
// --help-summary. A plugin that writes more than this is not
// answering the question, and the description ends up in a state file
// every tai invocation parses, so the bound is enforced rather than
// silently truncated.
const maxHelpSummaryBytes = 1024

// helpSummaryTimeout bounds the wire-verb subprocess. A plugin that
// hangs here would otherwise hang the install with no output; the
// verb is a single println, so seconds are generous.
const helpSummaryTimeout = 10 * time.Second

// RequireAssetsDir verifies that a freshly-fetched plugin bundle
// carries a top-level `assets/` directory.
//
// The directory is mandatory but may be empty: a pure-binary plugin
// ships no skills, commands, or agents and still declares that the
// host owns asset placement. Its absence signals a plugin that
// expects to write to target directories itself, which the wire
// contract forbids — SyncAssetsToTargets is the only sanctioned
// writer, and `assets/` is its guaranteed input.
//
// Called before namespace validation and before anything is copied
// anywhere, so a rejected bundle leaves no trace beyond the staging
// directory the caller already cleans up.
func RequireAssetsDir(bundleDir, name string) error {
	info, err := os.Stat(filepath.Join(bundleDir, "assets"))
	if err == nil && info.IsDir() {
		return nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errcode.Wrapf(errcode.InternalError, err,
			"stat assets directory of plugin %s", name)
	}
	return errcode.Newf(errcode.PluginAssetMissing,
		"plugin %s ships no top-level assets/ directory", name).
		WithHelp(
			"the plugin's release tarball must contain an `assets/` directory",
			"an empty `assets/` is valid — a plugin that ships no skills, commands, or agents still needs it",
			"tai installs assets itself; a plugin must not write to target directories from its own subcommands",
		)
}

// ReadHelpSummary runs the staged plugin binary's `--help-summary`
// wire verb and returns the single-line description to record in
// plugins.json.
//
// The plugin must exit zero and write a non-empty line. Anything else
// — a non-zero exit, silence, whitespace only, or more than 1 KB —
// fails with PLUGIN_HELP_SUMMARY_FAILED. Callers run this before
// promoting the staged directory, so a plugin that cannot answer
// never replaces a working prior install.
//
// Only the first line is kept, trimmed: a plugin that prints a
// paragraph gets its headline stored rather than a rejection.
func ReadHelpSummary(ctx context.Context, bundleDir, name string) (string, error) {
	binPath := filepath.Join(bundleDir, binaryName(name))

	ctx, cancel := context.WithTimeout(ctx, helpSummaryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, taiplugin.HelpSummaryFlag)
	cmd.Stdin = nil
	// A helper the plugin spawns could hold the stdout pipe open past
	// the deadline; WaitDelay unblocks the wait shortly after.
	cmd.WaitDelay = time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", errcode.Wrapf(errcode.PluginHelpSummaryFailed, err,
			"plugin %s failed to answer %s", name, taiplugin.HelpSummaryFlag).
			WithHelp(
				"every plugin must implement the `"+taiplugin.HelpSummaryFlag+"` wire verb: print one line describing itself, then exit zero",
				"Go plugins get it from `taiplugin.HelpSummary(os.Stdout, os.Args, \"<description>\")`",
				trimmedDetail(stderr.String()),
			)
	}

	if stdout.Len() > maxHelpSummaryBytes {
		return "", errcode.Newf(errcode.PluginHelpSummaryFailed,
			"plugin %s wrote %d bytes for %s, over the %d-byte limit",
			name, stdout.Len(), taiplugin.HelpSummaryFlag, maxHelpSummaryBytes).
			WithHelp(
				"the summary must be a single line of 80 characters or fewer",
				"tai stores it in plugins.json and shows it in `tai --help`",
			)
	}

	line, _, _ := strings.Cut(stdout.String(), "\n")
	line = strings.TrimSpace(line)
	if line == "" {
		return "", errcode.Newf(errcode.PluginHelpSummaryFailed,
			"plugin %s wrote nothing for %s", name, taiplugin.HelpSummaryFlag).
			WithHelp(
				"the plugin must print one line describing itself, then exit zero",
				"Go plugins get it from `taiplugin.HelpSummary(os.Stdout, os.Args, \"<description>\")`",
			)
	}
	return line, nil
}

// binaryName returns the plugin's executable filename for the host
// platform. The plugin's directory name is its binary name, plus the
// Windows suffix.
func binaryName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// trimmedDetail renders a subprocess's stderr as a help bullet,
// collapsed to one line and bounded so a chatty plugin cannot flood
// the user's terminal through tai's error template.
func trimmedDetail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "the plugin wrote nothing to stderr explaining the failure"
	}
	if line, _, found := strings.Cut(s, "\n"); found {
		s = line + " …"
	}
	if len(s) > 200 {
		s = s[:200] + " …"
	}
	return "the plugin reported: " + s
}
