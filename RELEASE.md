# Release Cycle

Operator's manual for cutting a release of `tai` (core) or a first-party plugin (currently just `triage`). The contract these commands implement is pinned in `openspec/specs/release-cycle/spec.md`. Anything surprising should be answered there; this file is the runbook.

---

## TL;DR

```bash
# Core release
git tag v0.6.0
git push origin v0.6.0
make release-core

# Triage plugin release
git tag plugins/triage/v0.5.0
git push origin plugins/triage/v0.5.0
make release-triage

# Dry-run both configs before tagging
make release-snapshot
```

---

## Prerequisites

### One-time

- **GoReleaser ≥ v1.13** on `$PATH` (the `monorepo.tag_prefix` feature lands in v1.13).
  - macOS: `brew install goreleaser/tap/goreleaser`
  - Linux: see <https://goreleaser.com/install/>
- **A self-hosted Homebrew tap repo** at `github.com/dmastrorillo/homebrew-tap`. Create it empty — the first `make release-core` populates it. A one-line README mentioning "auto-managed by GoReleaser" is enough.
- **A `HOMEBREW_TAP_GITHUB_TOKEN`** in your shell env. Generate a fine-grained PAT with **Contents: Write** scope limited to `dmastrorillo/homebrew-tap`. Required for core releases only (plugin releases don't push to the tap).

### Per-release

- A clean working tree (GoReleaser refuses to run if `git status` is dirty).
- `GITHUB_TOKEN` exported in your shell with write access to the `dmastrorillo/tai` repo.
- The HEAD commit at a properly-formatted tag (see Tag conventions below).

---

## Tag conventions

| What you're releasing | Tag pattern         | Example                    |
|-----------------------|---------------------|----------------------------|
| Core (`tai`) binary   | `vX.Y.Z`            | `v0.6.0`                   |
| Triage plugin         | `plugins/triage/vX.Y.Z` | `plugins/triage/v0.5.0` |

The asymmetry is real and load-bearing — see `openspec/changes/release-cycle/design.md` D1 (Go's module-version rule forces bare tags for core; plugins are not subject to that constraint and use prefixed tags so they can release independently on the same Releases page).

Pre-releases use the standard SemVer suffix: `v0.6.0-rc.1`, `plugins/triage/v0.5.0-beta.2`. GoReleaser auto-flags those as `prerelease: true` on the GitHub Release; the banner and the plugin host's "latest" lookups skip them. Users opt in explicitly via `--version` (plugins) or `@version` (`go install`).

---

## What each Make target does

### `make release-snapshot`

Runs both configs with `--snapshot --clean --skip=publish,announce`. Produces `dist/` archives without publishing or pushing. No env vars required. Use this to validate changes to `.goreleaser.*.yaml` before tagging.

### `make release-core`

Runs `goreleaser release --config .goreleaser.core.yaml --clean`. Publishes:

- A GitHub Release on `dmastrorillo/tai` at the current tag.
- Archives: `tai_<os>_<arch>.tar.gz` (Linux/macOS), `tai_windows_<arch>.zip`.
- `checksums.txt` alongside.
- An updated `Formula/tai.rb` committed and pushed to `dmastrorillo/homebrew-tap`.

Requires: `GITHUB_TOKEN`, `HOMEBREW_TAP_GITHUB_TOKEN`.

### `make release-triage`

Runs `goreleaser release --config .goreleaser.triage.yaml --clean`. Publishes:

- A GitHub Release on `dmastrorillo/tai` at the current `plugins/triage/vX.Y.Z` tag.
- One archive per `os × arch`: `tai-plugin-triage-<os>-<arch>.tar.gz` (always tar.gz, even on Windows — the plugin-host fetcher reads tarballs only).
- `checksums.txt` alongside.

Requires: `GITHUB_TOKEN`. NO tap token — plugins are not Homebrew-distributable.

---

## Validating the release worked

### After `make release-core`

```bash
# 1. Brew install fetches the new version
brew update
brew install dmastrorillo/tap/tai
tai --version   # must print v<X.Y.Z>, not "dev"

# 2. go install fetches the new version
GOBIN=$(mktemp -d) go install github.com/dmastrorillo/tai/core/cmd/tai@latest
$GOBIN/tai --version   # must print v<X.Y.Z>

# 3. The daily banner reflects the new latest within ~6 hours
#    (controlled by update-check-interval).
```

### After `make release-triage`

```bash
# 1. The plugin host picks up the new version
tai plugins triage install        # fresh install
tai plugins triage update         # if previously installed

# 2. The version is recorded in plugins.json
cat "$TAI_DATA_DIR/state/plugins.json" | jq '.plugins[] | select(.name=="triage")'
```

---

## What changes mid-cycle

Common operations during a release cycle (not the release itself):

- **Bump a dependency that affects both binaries.** Commit under `feat(pkg): ...` or `fix(pkg): ...`. The next release of EITHER binary picks it up — the auto-changelog includes `pkg`-scoped commits in both core's and triage's changelogs.
- **Add a new first-party plugin.** Add a new `.goreleaser.<plugin>.yaml` sibling to the existing two. Add a new Make target. Update the `scope-enum` in `.commitlintrc.yml` if the plugin's name should be a separate commit scope. Update `core/internal/plugins/registry.go`.
- **Cut a hotfix on an older minor.** Tag from the older minor's branch (e.g. `git tag v0.5.1` from a `release/0.5` branch). The auto-changelog diff-walks from the previous matching-prefix tag, so the hotfix changelog stays narrow.

---

## Future: CI port

The local-first design above is the v1 of the cycle. The intended CI port:

- `.github/workflows/release-core.yml` triggered by `on: push: tags: 'v*'`.
- `.github/workflows/release-triage.yml` triggered by `on: push: tags: 'plugins/triage/v*'`.
- Both run the same `goreleaser release --clean` command. `GITHUB_TOKEN` is auto-provided by Actions; `HOMEBREW_TAP_GITHUB_TOKEN` is stored as a repo secret.

CI port is OUT OF SCOPE for the initial release-cycle change. Land the local flow first, prove it, then port.
