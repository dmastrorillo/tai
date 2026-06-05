package plugins_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/plugins"
)

// releasesHandler returns an http.HandlerFunc that responds to
// `GET /repos/{repo}/releases?per_page=100` with a canned JSON list.
// The handler is permissive about query params — tests only need
// the path to match. Used by TestLatestPrefixed_* tests.
type releaseEntry struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
}

func releasesHandler(t *testing.T, repo string, entries []releaseEntry) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/repos/" + repo + "/releases"
		if r.URL.Path != wantPath {
			t.Errorf("unexpected path %q (want %q)", r.URL.Path, wantPath)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	}
}

// TestAssetFilename_TCREL001_matches_goreleaser_archive_name pins the
// release-asset filename contract owned by core/internal/plugins:
// the same string is consumed by the HTTP fetcher when looking up
// a plugin release's tarball and produced by the release pipeline's
// `.goreleaser.<plugin>.yaml`. Drift breaks `tai plugins <name>
// install` at runtime; this test catches it at test time.
//
// Spec: openspec/specs/release-cycle/spec.md, "GoReleaser
// configuration layout" → scenario "Triage archive name matches
// plugin-host expectation". TC-ID: TC-REL-001
// (core/test-cases.md).
//
// Build matrix mirrored from both `.goreleaser.core.yaml` and
// `.goreleaser.triage.yaml` (`goos: [linux, darwin, windows]`,
// `goarch: [amd64, arm64]`). If the matrix is widened (e.g.
// `arm/v7` lands), extend the table here too.
func TestAssetFilename_TCREL001_matches_goreleaser_archive_name(t *testing.T) {
	matrix := []struct {
		plugin string
		goos   string
		goarch string
		want   string
	}{
		{"triage", "linux", "amd64", "tai-plugin-triage-linux-amd64.tar.gz"},
		{"triage", "linux", "arm64", "tai-plugin-triage-linux-arm64.tar.gz"},
		{"triage", "darwin", "amd64", "tai-plugin-triage-darwin-amd64.tar.gz"},
		{"triage", "darwin", "arm64", "tai-plugin-triage-darwin-arm64.tar.gz"},
		{"triage", "windows", "amd64", "tai-plugin-triage-windows-amd64.tar.gz"},
		{"triage", "windows", "arm64", "tai-plugin-triage-windows-arm64.tar.gz"},
	}
	for _, tc := range matrix {
		t.Run(tc.plugin+"_"+tc.goos+"_"+tc.goarch, func(t *testing.T) {
			got := plugins.AssetFilename(tc.plugin, tc.goos, tc.goarch)
			if got != tc.want {
				t.Fatalf("AssetFilename(%q, %q, %q) = %q, want %q (drift breaks `tai plugins %s install` against the release built by .goreleaser.%s.yaml)",
					tc.plugin, tc.goos, tc.goarch, got, tc.want, tc.plugin, tc.plugin)
			}
		})
	}
}

// TestLatestPrefixed_TCREL002_filters_by_prefix exercises the
// requirement that the prefix-aware lookup ignores releases whose
// tag does not start with the given prefix. The fixture mixes core
// (unprefixed) and triage (prefixed) tags; the lookup MUST return
// the highest-semver triage tag and silently ignore the
// chronologically newer core tags.
//
// TC-ID: TC-REL-002 (core/test-cases.md).
func TestLatestPrefixed_TCREL002_filters_by_prefix(t *testing.T) {
	srv := httptest.NewServer(releasesHandler(t, "dmastrorillo/tai", []releaseEntry{
		// Listed newest-first by published_at (typical GitHub API
		// ordering). The algorithm MUST NOT honour publication order
		// — only the prefix and the parsed semver.
		{TagName: "v0.6.1"},
		{TagName: "plugins/triage/v0.5.0"},
		{TagName: "v0.6.0"},
		{TagName: "plugins/triage/v0.4.0"},
	}))
	defer srv.Close()

	got, err := plugins.LatestPrefixedTag(context.Background(), srv.Client(), srv.URL,
		"github.com", "dmastrorillo/tai", "plugins/triage/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "plugins/triage/v0.5.0" {
		t.Fatalf("got %q, want %q (algorithm must filter by prefix and return full tag, not stripped suffix)",
			got, "plugins/triage/v0.5.0")
	}
}

// TestLatestPrefixed_TCREL003_drops_prereleases verifies that a
// release flagged `prerelease: true` is skipped even when it would
// otherwise be the max-semver match.
//
// TC-ID: TC-REL-003.
func TestLatestPrefixed_TCREL003_drops_prereleases(t *testing.T) {
	srv := httptest.NewServer(releasesHandler(t, "dmastrorillo/tai", []releaseEntry{
		{TagName: "plugins/triage/v0.5.0-rc.1", Prerelease: true},
		{TagName: "plugins/triage/v0.4.0"},
	}))
	defer srv.Close()

	got, err := plugins.LatestPrefixedTag(context.Background(), srv.Client(), srv.URL,
		"github.com", "dmastrorillo/tai", "plugins/triage/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "plugins/triage/v0.4.0" {
		t.Fatalf("got %q, want %q (prereleases must be dropped)", got, "plugins/triage/v0.4.0")
	}

	// Empty-result sentinel: when the ONLY match is a prerelease,
	// the algorithm returns ("", nil), not an error.
	srv2 := httptest.NewServer(releasesHandler(t, "dmastrorillo/tai", []releaseEntry{
		{TagName: "plugins/triage/v0.5.0-rc.1", Prerelease: true},
	}))
	defer srv2.Close()

	got, err = plugins.LatestPrefixedTag(context.Background(), srv2.Client(), srv2.URL,
		"github.com", "dmastrorillo/tai", "plugins/triage/")
	if err != nil {
		t.Fatalf("unexpected error on empty-stable result: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string sentinel for 'no stable release'", got)
	}
}

// TestLatestPrefixed_TCREL004_picks_max_semver verifies that the
// returned tag is the highest semver, NOT the chronologically newest.
//
// TC-ID: TC-REL-004.
func TestLatestPrefixed_TCREL004_picks_max_semver(t *testing.T) {
	srv := httptest.NewServer(releasesHandler(t, "dmastrorillo/tai", []releaseEntry{
		// v0.4.1 is listed first (chronologically newest e.g. a
		// hotfix on a previous line). v0.5.0 is the correct answer
		// because it has the highest semver.
		{TagName: "plugins/triage/v0.4.1"},
		{TagName: "plugins/triage/v0.5.0"},
		{TagName: "plugins/triage/v0.4.0"},
	}))
	defer srv.Close()

	got, err := plugins.LatestPrefixedTag(context.Background(), srv.Client(), srv.URL,
		"github.com", "dmastrorillo/tai", "plugins/triage/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "plugins/triage/v0.5.0" {
		t.Fatalf("got %q, want %q (algorithm must pick max-semver, not most-recently-published)",
			got, "plugins/triage/v0.5.0")
	}
}

// TestLatestPrefixed_TCREL005_tolerates_malformed_tags verifies that
// a tag whose prefix-stripped suffix fails to parse as semver is
// silently dropped — not surfaced as an error.
//
// TC-ID: TC-REL-005.
func TestLatestPrefixed_TCREL005_tolerates_malformed_tags(t *testing.T) {
	srv := httptest.NewServer(releasesHandler(t, "dmastrorillo/tai", []releaseEntry{
		{TagName: "plugins/triage/v0.5.0"},
		{TagName: "plugins/triage/oops-not-a-version"},
	}))
	defer srv.Close()

	got, err := plugins.LatestPrefixedTag(context.Background(), srv.Client(), srv.URL,
		"github.com", "dmastrorillo/tai", "plugins/triage/")
	if err != nil {
		t.Fatalf("unexpected error on malformed tag: %v", err)
	}
	if got != "plugins/triage/v0.5.0" {
		t.Fatalf("got %q, want %q (malformed tags must be silently dropped)",
			got, "plugins/triage/v0.5.0")
	}
}

// TestPluginTagPrefix exercises the registry-lookup branch of
// PluginTagPrefix. The first-party match returns the prefixed form;
// any mismatch (unknown name, registered name but wrong repo) returns
// the empty string. Not tied to a TC-ID — this is a direct unit
// test for an internal helper exercised only indirectly via banner
// integration elsewhere.
func TestPluginTagPrefix(t *testing.T) {
	cases := []struct {
		name string
		src  plugins.Source
		want string
	}{
		{
			name: "triage",
			src:  plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
			want: "plugins/triage/",
		},
		{
			name: "triage",
			src:  plugins.Source{Host: "github.com", Repo: "acme/fork"},
			want: "",
		},
		{
			name: "unknown-plugin",
			src:  plugins.Source{Host: "github.com", Repo: "acme/whatever"},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+"_"+tc.src.Repo, func(t *testing.T) {
			got := plugins.PluginTagPrefix(tc.name, tc.src)
			if got != tc.want {
				t.Errorf("PluginTagPrefix(%q, %+v) = %q, want %q", tc.name, tc.src, got, tc.want)
			}
		})
	}
}
