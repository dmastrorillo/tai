package cmd_test

import (
	"strings"
	"testing"
)

// A plugin owns its entire argument surface. The host resolves the
// plugin name and forwards everything after it verbatim — it must not
// try to interpret a plugin's flags against its own flag set, because
// it has no way to know them.
//
// Before this was fixed, urfave/cli parsed the root command's flags
// before the dispatch Action ran, so `tai triage list --pr 99` died
// with `flag provided but not defined: -pr` and never reached the
// plugin. Since nearly every plugin verb takes a flag, the plugin was
// effectively unreachable through the host.
func TestPluginInvoke_TCPLG023_flags_pass_through_verbatim(t *testing.T) {
	dataDir := pluginsEnv(t)
	// Echo every argument on its own line so the test can assert on
	// the exact argv the plugin received, in order.
	installFakePlugin(t, dataDir, "triage",
		`for a in "$@"; do printf '%s\n' "$a"; done
`)

	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "flag after the verb",
			argv: []string{"triage", "list", "--pr", "99"},
			want: []string{"list", "--pr", "99"},
		},
		{
			name: "global plugin flag before the verb",
			argv: []string{"triage", "--repo", "acme/demo", "list"},
			want: []string{"--repo", "acme/demo", "list"},
		},
		{
			name: "flag the host also defines is not intercepted",
			argv: []string{"triage", "list", "--help"},
			want: []string{"list", "--help"},
		},
		{
			name: "bare --help reaches the plugin, not the host",
			argv: []string{"triage", "--help"},
			want: []string{"--help"},
		},
		{
			name: "value that looks like a flag survives",
			argv: []string{"triage", "dismiss", "--reason", "--not-a-flag"},
			want: []string{"dismiss", "--reason", "--not-a-flag"},
		},
		{
			name: "no arguments at all",
			argv: []string{"triage"},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runRoot(t, tc.argv...)
			if r.err != nil {
				t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
			}
			var got []string
			if s := strings.TrimRight(r.stdout, "\n"); s != "" {
				got = strings.Split(s, "\n")
			}
			if len(got) != len(tc.want) {
				t.Fatalf("plugin received %q, want %q", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("argv[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// The host's own verbs keep their flag parsing — the passthrough must
// be scoped to installed plugins, not global.
func TestPluginInvoke_TCPLG023_host_verbs_still_parse_flags(t *testing.T) {
	pluginsEnv(t)

	r := runRoot(t, "--version")
	if r.err != nil {
		t.Fatalf("tai --version must still work: %v", r.err)
	}
	if !strings.Contains(r.stdout, "tai version") {
		t.Errorf("stdout = %q, want a version line", r.stdout)
	}

	// An unknown flag on the host is still a usage error, not a
	// silent passthrough to nowhere.
	if bad := runRoot(t, "--definitely-not-a-flag"); bad.err == nil {
		t.Error("an unknown host flag must still be rejected")
	}
}
