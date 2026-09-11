package cmd_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmd"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
)

// staleGoInvocation matches a triage verb addressed the way it was
// when triage lived inside the core binary. Those verbs moved out
// when triage became a plugin, so the bare form now fails with
// UNKNOWN_SUBCOMMAND and only the plugin-qualified form runs.
//
// The qualified form contains no bare-form substring — `triage` sits
// between the two words — so no negative lookahead is needed, which
// is the construct Go's regexp lacks.
var staleGoInvocation = regexp.MustCompile(`\btai (status|list|show|accept|dismiss|complete|forget|import)\b`)

// TC-TRG-106 — the plugin's own help text and doc comments address its
// verbs by the name that runs.
//
// Error help is the CLI's most load-bearing prose: it is read exactly
// when the user is already stuck. Resolving a scope outside a repo
// pointed the reader at an import command spelled the pre-plugin way,
// which no longer exists. TC-AST-001 covers the shipped markdown
// under assets/ but cannot see the strings compiled into the binary,
// which is how these survived that pass.
func TestTriageSource_TCTRG106_addresses_verbs_as_tai_triage(t *testing.T) {
	root := filepath.Join("..", "..")

	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(body), "\n") {
			if m := staleGoInvocation.FindString(line); m != "" {
				offenders = append(offenders, filepath.ToSlash(path)+":"+strconv.Itoa(i+1)+"  "+m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(offenders) > 0 {
		t.Errorf("%d stale invocation(s) — these verbs live in the triage plugin, "+
			"so they must read `tai triage <verb>`:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TC-TRG-106 — the rendered stderr, not just the source.
//
// The walk above is the blanket net: it sees every Go file, so no call
// site can slip past it. What it cannot see is a message assembled at
// runtime, where the source carries no literal for the regex to match
// but the user still reads the wrong command. These two assertions
// cover the highest-traffic corrected strings at the layer the user
// actually meets them.
func TestTriageErrors_TCTRG106_render_the_plugin_invocation(t *testing.T) {
	cmdtest.Isolate(t)
	cmdtest.Chdir(t, t.TempDir())

	// `forget` with no selector — deliberately not via the triage()
	// helper, which auto-prepends --repo and would turn this into the
	// repo-selector mode.
	forget := cmdtest.Run(t, cmd.NewRoot(), "forget")
	cmdtest.AssertError(t, forget)
	assertNoStaleInvocation(t, "forget", forget.Stderr)

	show := cmdtest.Run(t, cmd.NewRoot(), "show")
	cmdtest.AssertError(t, show)
	assertNoStaleInvocation(t, "show", show.Stderr)
}

// assertNoStaleInvocation fails when rendered output addresses a
// triage verb as a bare `tai <verb>`, and requires that it names the
// plugin form at least once — so a message that simply stopped
// mentioning any command cannot pass by omission.
func assertNoStaleInvocation(t *testing.T, verb, stderr string) {
	t.Helper()
	if stderr == "" {
		t.Fatalf("%s: expected an error message on stderr, got nothing", verb)
	}
	if m := staleGoInvocation.FindString(stderr); m != "" {
		t.Errorf("%s: rendered stderr addresses a verb as %q:\n%s", verb, m, stderr)
	}
	if !strings.Contains(stderr, "tai triage ") {
		t.Errorf("%s: rendered stderr must name the plugin form `tai triage <verb>`:\n%s", verb, stderr)
	}
}
