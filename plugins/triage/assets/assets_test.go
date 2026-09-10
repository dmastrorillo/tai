// Package assets_test pins the content contract of the markdown the
// triage plugin ships for installation into target directories.
//
// The host copies `assets/commands/*.md` into
// `<target>/commands/tai-triage/`, which is what makes them reachable
// as `/tai-triage:<verb>`. A file that tells the reader to invoke
// itself under any other name sends them to a command that does not
// exist, and nothing else in the pipeline would catch it — the host
// copies bytes without reading them.
package assets_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// commandsDir is the tree the plugin tarball ships and the host
// copies from.
const commandsDir = "commands"

// staleRef matches a slash-command reference in the pre-plugin-host
// namespace: `/tai:<verb>`. The host now routes these files into a
// `tai-triage/` subdirectory, so `/tai-triage:<verb>` is the only
// name that resolves.
var staleRef = regexp.MustCompile(`/tai:[a-z-]+`)

// staleInvocation matches a CLI invocation in the pre-plugin-host
// form: `tai <triage-verb>`. These verbs moved out of the core binary
// when triage became a plugin, so the only form that runs today is
// `tai triage <verb>`. The negative lookahead Go's regexp lacks is
// unnecessary here — `tai triage status` contains no `tai status`
// substring, since `triage` sits between them.
//
// Checked separately from staleRef because the two drifted
// independently: an earlier pass fixed every `/tai:` reference and
// left all 58 CLI invocations behind, which this test could not see.
var staleInvocation = regexp.MustCompile(`\btai (status|list|show|accept|dismiss|complete|forget|import)\b`)

// TC-AST-001 — every bundled command addresses itself by its
// installed slash-command name.
func TestBundledCommands_TCAST001_use_the_plugin_namespace(t *testing.T) {
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		checked++
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(commandsDir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			body := string(data)
			if found := staleRef.FindAllString(body, -1); len(found) > 0 {
				t.Errorf("%d stale slash-command reference(s): %v\n"+
					"the host installs these under `tai-triage/`, so they must read `/tai-triage:<verb>`",
					len(found), unique(found))
			}
			if found := staleInvocation.FindAllString(body, -1); len(found) > 0 {
				t.Errorf("%d stale CLI invocation(s): %v\n"+
					"these verbs live in the triage plugin, so they must read `tai triage <verb>`",
					len(found), unique(found))
			}
		})
	}

	// A rename or a move that empties the tree would otherwise let
	// this test pass by checking nothing.
	if checked == 0 {
		t.Fatalf("no command markdown found in %s/ — the tarball ships this tree", commandsDir)
	}
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
