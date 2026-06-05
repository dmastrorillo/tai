package plugins

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// LatestPrefixedTag implements the prefix-aware latest-release
// lookup against the GitHub Releases API — the algorithm pinned by
// the `release-cycle` capability spec (Requirement: Prefix-aware
// latest release lookup). Both the plugin host's fetcher (when
// `--version` is omitted) and the update banner's plugin-row layer
// call it.
//
// The shape — list, filter by prefix, drop prereleases, parse the
// suffix as semver, return the max — is deliberately different
// from the GitHub `/releases/latest` endpoint, which returns the
// chronologically newest non-prerelease release REGARDLESS of tag
// prefix. Under prefixed plugin tags, the /latest endpoint would
// cross-contaminate plugin lookups with core releases. See
// design.md D5 for the full rationale.
//
// Returns the FULL `tag_name` of the highest stable semver release
// whose tag starts with `prefix` — e.g. "plugins/triage/v0.5.0",
// NOT "v0.5.0". The fetcher passes the full tag straight to
// `/releases/tags/<tag>`; the banner caller strips the prefix for
// display.
//
// Return shape:
//
//   - (fullTag, nil)        — a matching stable release was found.
//     fullTag is the unmodified `tag_name` from the GitHub API
//     payload (i.e. includes the prefix).
//   - ("", nil)             — no matching stable release. Callers
//     MUST treat this as "no update available", NOT an error. This
//     matches the spec's "no release sentinel".
//   - ("", *errcode.Error)  — a real lookup failure (network, 5xx,
//     401/403, malformed JSON).
//
// Parameters:
//
//   - client       — HTTP client. Caller-supplied so production can
//     bind a timeout-bounded client and tests can inject
//     httptest.Server.Client().
//   - baseURL      — GitHub API base. Production passes
//     "https://api.github.com"; tests pass an httptest server URL.
//   - host         — release host shortcode. Only "github.com" is
//     supported today; other values surface as PLUGIN_FETCH_FAILED.
//   - repo         — "<org>/<repo>" path. The fetch URL is
//     `{baseURL}/repos/{repo}/releases?per_page=100`.
//   - prefix       — the plugin's tag prefix (e.g.
//     "plugins/triage/"). Empty string means "match every tag",
//     used for third-party plugins whose source repo carries no
//     prefix.
//
// The per_page=100 ceiling is a documented limit — see design.md
// (Risks / Trade-offs). 100 is generous for any plugin stream
// inside one repo for years; if we hit it, the lookup degrades to
// "miss the latest stable" rather than crash.
func LatestPrefixedTag(ctx context.Context, client *http.Client, baseURL, host, repo, prefix string) (string, error) {
	if host != "github.com" {
		return "", errcode.Newf(errcode.PluginFetchFailed,
			"unsupported release host %q — only github.com is supported", host)
	}
	if repo == "" {
		return "", errcode.New(errcode.PluginFetchFailed,
			"empty repo for prefix-aware lookup")
	}
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	endpoint := baseURL + "/repos/" + repo + "/releases?per_page=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", errcode.Wrapf(errcode.PluginFetchFailed, err,
			"build request for %s", endpoint)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	authorizeRequest(req)

	resp, err := client.Do(req)
	if err != nil {
		return "", errcode.Wrapf(errcode.PluginFetchFailed, err,
			"GET %s", endpoint).
			WithHelp("check your network connection and retry")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", errcode.Newf(errcode.PluginFetchUnauthorized,
			"GET %s: HTTP %d", endpoint, resp.StatusCode).
			WithHelp("set GITHUB_TOKEN to authenticate against the source repo")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", errcode.Newf(errcode.PluginFetchFailed,
			"GET %s: HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", errcode.Wrapf(errcode.PluginFetchFailed, err,
			"parse %s", endpoint)
	}

	bestTag := ""
	bestMajor, bestMinor, bestPatch := -1, -1, -1
	for _, e := range payload {
		if e.Prerelease {
			continue
		}
		if !strings.HasPrefix(e.TagName, prefix) {
			continue
		}
		stripped := strings.TrimPrefix(e.TagName, prefix)
		maj, min, patch, ok := parseSemverNumeric(stripped)
		if !ok {
			// Malformed suffix (e.g. "oops-not-a-version"). Silent
			// drop per TC-REL-005 — not surfaced as an error.
			continue
		}
		if compareSemver(maj, min, patch, bestMajor, bestMinor, bestPatch) > 0 {
			bestMajor, bestMinor, bestPatch = maj, min, patch
			// Return the FULL tag_name (e.g.
			// "plugins/triage/v0.5.0"); the fetcher needs it for
			// `/releases/tags/<tag>`. Banner callers strip the
			// prefix themselves for display.
			bestTag = e.TagName
		}
	}
	return bestTag, nil
}

// parseSemverNumeric parses `vMAJ.MIN.PATCH` (with leading `v`,
// no pre-release suffix, no build metadata). Returns ok=false for
// anything that doesn't match exactly. Pre-release tags (those
// containing `-`) are rejected here too as a defence in depth —
// LatestPrefixedTag already filters them via the JSON `prerelease`
// field, but a tag mismarked by the publisher would still be
// dropped here.
func parseSemverNumeric(s string) (major, minor, patch int, ok bool) {
	if !strings.HasPrefix(s, "v") {
		return 0, 0, 0, false
	}
	s = s[1:]
	// Reject build metadata (`+`) and pre-release (`-`) suffixes.
	if strings.ContainsAny(s, "+-") {
		return 0, 0, 0, false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var err error
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, false
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, 0, false
	}
	if patch, err = strconv.Atoi(parts[2]); err != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

// compareSemver returns -1, 0, or +1 in the usual sense for two
// (major, minor, patch) tuples. Used as the picker for the
// maximum across the filtered release list.
func compareSemver(amaj, amin, apat, bmaj, bmin, bpat int) int {
	if amaj != bmaj {
		if amaj > bmaj {
			return 1
		}
		return -1
	}
	if amin != bmin {
		if amin > bmin {
			return 1
		}
		return -1
	}
	if apat != bpat {
		if apat > bpat {
			return 1
		}
		return -1
	}
	return 0
}
