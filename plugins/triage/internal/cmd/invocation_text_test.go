package cmd_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
				offenders = append(offenders, filepath.ToSlash(path)+":"+itoa(i+1)+"  "+m)
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
