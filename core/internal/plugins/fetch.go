package plugins

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// Fetcher resolves a Source to a populated `<TAI_DATA_DIR>/plugins/
// <name>/` directory: the binary + `assets/`. Implementations differ
// only in HOW the asset bytes are obtained — production uses HTTP
// against GitHub Releases; tests inject a local-file fetcher.
type Fetcher interface {
	// Fetch downloads the release asset for `pluginName` at the
	// given source, unpacks it into `destDir` (creating destDir
	// fresh — caller MUST pass an empty directory), and returns the
	// resolved version string (the exact tag fetched).
	Fetch(ctx context.Context, pluginName string, src Source, destDir string) (string, error)
}

// HTTPFetcher is the production fetcher. Pulls `tai-plugin-<name>-
// <os>-<arch>.tar.gz` from the host's Releases API and unpacks the
// tarball into destDir.
type HTTPFetcher struct {
	// Client is the HTTP client used for both the release-metadata
	// lookup and the asset download. Nil falls back to
	// `http.DefaultClient`. Tests inject an httptest-backed client.
	Client *http.Client

	// GitHubBaseURL overrides the GitHub API base. Production leaves
	// it empty (defaults to `https://api.github.com`); tests point
	// at an httptest server.
	GitHubBaseURL string
}

// Fetch implements the Fetcher contract against the GitHub Releases
// API. Steps:
//
//  1. Look up the release for src.Version (or "latest" if empty).
//  2. Find the asset matching `tai-plugin-<name>-<os>-<arch>.tar.gz`.
//  3. Download the asset (`GITHUB_TOKEN` Bearer when set).
//  4. Unpack the tarball into destDir.
//  5. Return the resolved tag.
//
// Errors are surfaced as `*errcode.Error` with the codes the spec
// pins: PLUGIN_FETCH_UNAUTHORIZED for 401/403; PLUGIN_FETCH_FAILED
// for everything else.
func (h *HTTPFetcher) Fetch(ctx context.Context, pluginName string, src Source, destDir string) (string, error) {
	if src.Host != "github.com" {
		return "", errcode.Newf(errcode.PluginFetchFailed,
			"unsupported plugin source host %q — only github.com is supported", src.Host).
			WithHelp("set the plugin source's host to github.com")
	}
	client := h.Client
	if client == nil {
		client = http.DefaultClient
	}
	base := h.GitHubBaseURL
	if base == "" {
		base = "https://api.github.com"
	}

	// Resolve the release tag.
	tag := src.Version
	assetURL, resolvedTag, err := h.lookupAsset(ctx, client, base, pluginName, src.Repo, tag)
	if err != nil {
		return "", err
	}

	// Download the tarball.
	body, err := h.downloadAsset(ctx, client, assetURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()

	// Unpack into destDir.
	if err := unpackTarGz(body, destDir); err != nil {
		return "", errcode.Wrapf(errcode.PluginFetchFailed, err,
			"unpack %s: %s", pluginName, err).
			WithHelp("the release asset may be corrupted — retry, or check the release page")
	}
	return resolvedTag, nil
}

// AssetFilename returns the conventional release-asset name for
// (plugin, os, arch). Exported so the GitHub-side release pipeline
// (Phase 7 task 11.4) can be generated from the same source of truth.
func AssetFilename(pluginName, goos, goarch string) string {
	return fmt.Sprintf("tai-plugin-%s-%s-%s.tar.gz", pluginName, goos, goarch)
}

// expectedAssetName is the platform-specific filename to look for
// in the release manifest.
func expectedAssetName(pluginName string) string {
	return AssetFilename(pluginName, runtime.GOOS, runtime.GOARCH)
}

// lookupAsset asks the GitHub Releases API for the given tag (or
// "latest"), iterates the assets, and returns the matching asset's
// download URL plus the resolved tag string.
func (h *HTTPFetcher) lookupAsset(ctx context.Context, client *http.Client, base, pluginName, repo, tag string) (string, string, error) {
	// `repo` is `<org>/<repo>` from the registry/source spec.
	if repo == "" {
		return "", "", errcode.New(errcode.PluginFetchFailed,
			"plugin source has no repo — expected <org>/<repo>").
			WithHelp("set the source spec to `<host>/<org>/<repo>[/<subpath>]@<version>`")
	}
	endpoint := base + "/repos/" + repo + "/releases/"
	if tag == "" || tag == "latest" {
		endpoint += "latest"
	} else {
		endpoint += "tags/" + tag
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	authorizeRequest(req)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", errcode.Wrapf(errcode.PluginFetchFailed, err,
			"GET %s: %s", endpoint, err).
			WithHelp("check your network connection and retry")
	}
	defer func() { _ = resp.Body.Close() }()

	if err := classifyHTTPStatus(resp); err != nil {
		return "", "", err
	}

	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", errcode.Wrapf(errcode.PluginFetchFailed, err,
			"parse %s: %s", endpoint, err)
	}

	want := expectedAssetName(pluginName)
	for _, a := range payload.Assets {
		if a.Name == want {
			return a.URL, payload.TagName, nil
		}
	}
	return "", "", errcode.Newf(errcode.PluginFetchFailed,
		"release %s of %s has no asset %q", payload.TagName, repo, want).
		WithHelp(
			"the plugin maintainer must publish "+want+" on this release",
			"or, if the release is intentional, try `--version` of a release that does ship the asset",
		)
}

func (h *HTTPFetcher) downloadAsset(ctx context.Context, client *http.Client, url string) (io.ReadCloser, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/octet-stream")
	authorizeRequest(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, errcode.Wrapf(errcode.PluginFetchFailed, err,
			"GET %s: %s", url, err).
			WithHelp("check your network connection and retry")
	}
	if err := classifyHTTPStatus(resp); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	return resp.Body, nil
}

// authorizeRequest attaches `Authorization: Bearer $GITHUB_TOKEN`
// when the env var is set and non-empty. Anonymous otherwise.
func authorizeRequest(req *http.Request) {
	tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
}

// classifyHTTPStatus maps the HTTP response status to the spec's
// error-code taxonomy. 2xx → nil. 401/403 → PLUGIN_FETCH_UNAUTHORIZED.
// Everything else → PLUGIN_FETCH_FAILED.
func classifyHTTPStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errcode.Newf(errcode.PluginFetchUnauthorized,
			"%s %s returned %d", resp.Request.Method, resp.Request.URL, resp.StatusCode).
			WithHelp(
				"set GITHUB_TOKEN to a personal access token with `repo` scope (private repos)",
				"or `public_repo` scope (public repos) if you're rate-limited",
			)
	}
	return errcode.Newf(errcode.PluginFetchFailed,
		"%s %s returned %d", resp.Request.Method, resp.Request.URL, resp.StatusCode).
		WithHelp(
			"retry — transient 5xx errors are common on release hosts",
			"if the failure persists, check the release page in a browser",
		)
}

// unpackTarGz reads a gzip'd tar stream from r and unpacks every
// regular file into destDir. Refuses path traversal (`..`) and
// absolute paths. Sets executable bit on the binary (entries at the
// top level whose mode includes any executable bit).
//
// destDir is assumed to be empty before this call; the caller is
// responsible for creating it.
func unpackTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip header: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		clean, ok := safeJoin(destDir, hdr.Name)
		if !ok {
			return fmt.Errorf("refusing tar entry with unsafe path %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(clean, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", clean, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
				return fmt.Errorf("mkdir for %s: %w", clean, err)
			}
			mode := os.FileMode(hdr.Mode & 0o777)
			if mode == 0 {
				mode = 0o644
			}
			f, err := os.OpenFile(clean, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return fmt.Errorf("create %s: %w", clean, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return fmt.Errorf("write %s: %w", clean, err)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("close %s: %w", clean, err)
			}
		default:
			// Skip symlinks, devices, etc. — tai's bundle format is
			// flat files + directories; anything else is unexpected.
			continue
		}
	}
}

// safeJoin joins root + rel and refuses paths that would escape
// root (via absolute paths or `..` traversal). Returns the cleaned
// path and ok=true when the result stays inside root.
func safeJoin(root, rel string) (string, bool) {
	if filepath.IsAbs(rel) {
		return "", false
	}
	full := filepath.Join(root, rel)
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	cleanFull, err := filepath.Abs(full)
	if err != nil {
		return "", false
	}
	if !strings.HasPrefix(cleanFull, cleanRoot+string(filepath.Separator)) && cleanFull != cleanRoot {
		return "", false
	}
	return full, true
}
