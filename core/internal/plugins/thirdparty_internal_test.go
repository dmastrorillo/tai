package plugins

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

var acme = Source{Host: "github.com", Repo: "acme/tai-plugin-acme"}

// TC-PLG-037 — in a terminal the user is asked, and the question says
// what they are agreeing to.
func TestConfirmThirdParty_TCPLG037_interactive_yes(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "  yes  \n"} {
		var stderr bytes.Buffer
		err := confirmThirdParty("acme", acme, strings.NewReader(answer), &stderr, false, true)
		if err != nil {
			t.Errorf("answer %q must proceed, got %v", answer, err)
		}
		prompt := stderr.String()
		for _, want := range []string{"acme", "github.com/acme/tai-plugin-acme", "arbitrary code", "[y/N]"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("prompt must contain %q, got %q", want, prompt)
			}
		}
	}
}

// TC-PLG-037 — anything that is not a yes is a no, including the
// empty answer a bare Enter produces.
func TestConfirmThirdParty_TCPLG037_interactive_no(t *testing.T) {
	for _, answer := range []string{"n\n", "N\n", "\n", "", "no\n", "sure\n"} {
		var stderr bytes.Buffer
		err := confirmThirdParty("acme", acme, strings.NewReader(answer), &stderr, false, true)
		if err == nil {
			t.Errorf("answer %q must refuse", answer)
			continue
		}
		coded, ok := errcode.As(err)
		if !ok || coded.Code != errcode.PluginThirdpartyUnconfirmed {
			t.Errorf("answer %q: want PLUGIN_THIRDPARTY_UNCONFIRMED, got %v", answer, err)
		}
	}
}

// A built-in plugin is never gated, whatever the terminal looks like.
func TestConfirmThirdParty_TCPLG035_builtin_is_never_gated(t *testing.T) {
	var stderr bytes.Buffer
	src, _ := Lookup("triage")
	if err := confirmThirdParty("triage", src, strings.NewReader(""), &stderr, false, false); err != nil {
		t.Fatalf("a built-in plugin must install without consent: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("a built-in plugin must print no prompt, got %q", stderr.String())
	}
}
