---
name: tai-release
description: Cuts a release of the tai CLI or one of its first-party plugins via the local-first goreleaser + gh-CLI pipeline owned by the release-cycle capability. Use when the user says "cut a release", "ship X", "tag and release", "release tai", "release triage", "publish v0.x", mentions `make release-snapshot` / `make release-core` / `make release-triage`, when working with `.goreleaser.core.yaml` / `.goreleaser.triage.yaml`, when validating the brew tap at `dmastrorillo/homebrew-tap`, when troubleshooting `tai plugins install/update triage` failures that may trace to a missing release asset, when handling the `plugins/<name>/vX.Y.Z` prefixed tag scheme, when adding a second first-party plugin, or when debugging why `tai --version` prints `dev` after a build. Also use for questions about the OSS workaround replacing goreleaser's Pro-only `monorepo.tag_prefix`.
---

# tai release lifecycle

Operator guide for cutting a release of the `tai` core CLI or the `triage` first-party plugin from the `tai` monorepo.

Canonical reference: `RELEASE.md` at repo root. Behavioural contract: `openspec/changes/release-cycle/specs/release-cycle/spec.md` (`openspec/specs/release-cycle/spec.md` after archive). Rationale: `openspec/changes/release-cycle/design.md` (D1 tag scheme, D2 OSS workaround for goreleaser Pro features).

## What this pipeline ships

| Path | Target | Tag | GitHub Release | Install surface |
|---|---|---|---|---|
| Core CLI | `make release-core` | `vX.Y.Z` (bare, at repo root) | At `vX.Y.Z` | `brew install dmastrorillo/tap/tai`, `go install github.com/dmastrorillo/tai/core/cmd/tai@latest`, direct download |
| Triage plugin | `make release-triage` | `plugins/triage/vX.Y.Z` | At full prefixed tag | `tai plugins install triage` only |

Plugins are NOT brew-distributable — the plugin host expects binaries under `<TAI_DATA_DIR>/plugins/<name>/`, not `/opt/homebrew/bin/`. Don't author a brew formula for triage.

## Prerequisites (one-time)

- **GoReleaser v2** on `$PATH`. `brew install goreleaser/tap/goreleaser`. OSS, no Pro license.
- **`gh` CLI** on `$PATH`. Used by `make release-triage` to publish at the prefixed tag (goreleaser v2 OSS can't do this natively — see "Why the shim").
- **`dmastrorillo/homebrew-tap`** GitHub repo, created empty. First `make release-core` populates it.
- **`HOMEBREW_TAP_GITHUB_TOKEN`** in shell env: fine-grained PAT with `Contents: Write` scoped to `dmastrorillo/homebrew-tap`. Core releases only.
- **`GITHUB_TOKEN`** in shell env: write scope on `dmastrorillo/tai`. Used by goreleaser and gh.

## Pre-release validation (always run first)

```bash
make release-snapshot
```

Produces archives under `dist/core/` (one per `os × arch`, plus the generated cask under `dist/core/homebrew/Casks/tai.rb`) and `dist/triage/` (same matrix, all `.tar.gz`). No env vars required.

Verify before tagging: extract a core archive, run `./tai --version`. Output must be `tai version v0.0.0-SNAPSHOT-<sha>` — not `dev` (means ldflags injection broke).

## Core release runbook

```bash
git tag v0.6.0
git push origin v0.6.0
make release-core
```

Validation:

```bash
brew update    # critical — fetches new tap formula
brew install dmastrorillo/tap/tai
tai --version  # must print v0.6.0

GOBIN=$(mktemp -d) go install github.com/dmastrorillo/tai/core/cmd/tai@latest
$GOBIN/tai --version
```

Goreleaser does the rest: builds the linux/darwin/windows × amd64/arm64 matrix with `CGO_ENABLED=0`, creates the GitHub Release at `v0.6.0`, uploads archives + `checksums.txt`, and pushes `Casks/tai.rb` to `dmastrorillo/homebrew-tap`. Per-step detail in `RELEASE.md`.

## Triage plugin release runbook

```bash
git tag plugins/triage/v0.5.0
git push origin plugins/triage/v0.5.0
make release-triage
```

Validation:

```bash
tai plugins install triage        # fresh install
tai plugins update triage         # if previously installed
cat "$TAI_DATA_DIR/state/plugins.json" | jq '.plugins[] | select(.name=="triage")'
```

`make release-triage` is a Make shim — two-step because goreleaser v2 OSS lacks `monorepo.tag_prefix` and `release.tag` (both Pro):

1. Validates HEAD tag matches `plugins/triage/v*`.
2. Runs `git ls-remote --exit-code origin "refs/tags/$tag"` to confirm the tag is pushed (fast-fail; build takes 10–30s).
3. Extracts bare semver: `bare=${tag#plugins/triage/}` → `v0.5.0`.
4. Runs `GORELEASER_CURRENT_TAG=v0.5.0 goreleaser release --config .goreleaser.triage.yaml --clean`. Config has `release: { disable: true }` — goreleaser BUILDS only.
5. Publishes via `gh release create plugins/triage/v0.5.0 --verify-tag dist/triage/tai-plugin-triage-*.tar.gz dist/triage/checksums.txt`. Pre-release flag branched via `case "$bare" in *-*) ... --prerelease ... ;;`.

## Pre-release tags

Use SemVer pre-release suffix: `v0.6.0-rc.1`, `plugins/triage/v0.5.0-beta.2`. Goreleaser auto-flags `prerelease: true` on the GitHub Release. Effects:

- Banner AND `tai plugins install <name>/update --version`-omitted path both ignore pre-releases (filter in `core/internal/plugins.LatestPrefixedTag`, called by both code paths).
- Brew tap cask is NOT updated (`skip_upload: auto`).

`LatestPrefixedTag` applies two independent pre-release filters: (1) the GitHub API's `prerelease: true` field; (2) `parseSemverNumeric` rejects any version string containing `-` or `+`. A tag mismarked as non-pre-release on the GitHub side is still dropped at step 2.

Opt-in paths:
- `tai plugins install <name> --version vX.Y.Z-rc.N`
- `go install github.com/dmastrorillo/tai/core/cmd/tai@vX.Y.Z-rc.N`
- Direct download from the GitHub Release page (visible, marked "Pre-release")

## Why the shim (goreleaser v2 Pro gap)

GoReleaser v2 OSS lost the ability to:
- Match tags by prefix (`monorepo.tag_prefix`) — Pro-only
- Override the release tag (`release.tag`) — Pro-only

Without these, a prefixed tag like `plugins/triage/v0.5.0` would break goreleaser's `.Version` computation (goreleaser strips a leading `v` from the tag → `.Version`; with `plugins/triage/v0.5.0` there's no leading `v`).

OSS workaround: `make release-triage` extracts the **bare semver** (e.g. `v0.5.0`, NOT the full prefixed tag) into `GORELEASER_CURRENT_TAG`. Goreleaser then strips the leading `v` and computes `.Version = 0.5.0` correctly. The ldflags template re-prefixes with `v{{ .Version }}` → injected binary version `v0.5.0`. Goreleaser BUILDS only (`release: { disable: true }`). `gh release create <full-prefixed-tag>` publishes at the original tag with the goreleaser-built archives.

Cost: ~20 lines of Make shell per plugin. Benefit: $0 license vs $19/mo for goreleaser Pro.

## Load-bearing contracts

These pieces are coupled — change one, change all.

1. **Archive filename**: `core/internal/plugins.AssetFilename(name, os, arch)` returns `tai-plugin-<name>-<os>-<arch>.tar.gz`. `.goreleaser.<plugin>.yaml`'s `archives.name_template` must produce byte-identical strings. Pinned by `TestAssetFilename_TCREL001_matches_goreleaser_archive_name`.

2. **Full-tag in `gh release create`**: `make release-triage` passes the full prefixed tag (`plugins/triage/v0.5.0`) to `gh release create` because `core/internal/plugins.LatestPrefixedTag` returns the full `tag_name`, and the fetcher then hits `/releases/tags/<full-tag>`. If a future Make edit passes the bare semver to `gh`, the fetcher will 404. The dependency is documented in `Fetch`'s doc comment.

3. **Linker injection target**: `.goreleaser.core.yaml` writes to `core/internal/version.String`; `.goreleaser.triage.yaml` writes to `plugins/triage/internal/version.String`. Two independent vars in separate packages — adding a third plugin needs a third version package.

4. **Cask `binaries: [tai]` → Ruby `binary "tai"` stanza**: the yaml key is `binaries: [tai]` (plural list — v2 syntax). Goreleaser writes that into the generated `Casks/tai.rb` as a Ruby `binary "tai"` stanza. Brew uses the stanza to symlink the binary into `/opt/homebrew/bin/tai`.

## Gotchas

- **Forgot to push the tag.** `make release-triage`'s `git ls-remote` pre-check catches this before goreleaser builds. `make release-core` relies on goreleaser's own tag check, which fails AFTER the matrix builds. Push the tag before either.
- **`brew install` serves the old version after a release.** Run `brew update` first to fetch the new tap commit. The Quick Reference command already includes it.
- **`v0.0.0` URLs in the snapshot cask.** Goreleaser uses `v0.0.0` as the synthetic snapshot version. Expected; real release tag replaces it.
- **`tai --version` prints `dev`.** Linker injection failed. Likely: `go build ./...` was used instead of goreleaser, OR an ldflags rewrite changed the `-X` target without updating `version.String`'s import path.
- **Goreleaser deprecation warnings.** v2.x is a moving target. Address before next cycle. Past migrations: `brews:` → `homebrew_casks:`, `archives.format` → `formats: [...]`, `homebrew_casks.binary` → `binaries`.
- **First-party plugin install fails with `PLUGIN_FETCH_FAILED` after a release.** Likely the asset filename in the GitHub Release doesn't match `AssetFilename`. Re-check `.goreleaser.<plugin>.yaml::archives.name_template`. TC-REL-001 pins the expected name but the YAML can drift.
- **Tap repo doesn't exist.** First `make release-core` fails with a confusing 404. Create `dmastrorillo/homebrew-tap` empty first.

## Changelog generation

Auto-generated by goreleaser from git commits between the current tag and the previous matching-prefix tag. Filter: `feat`, `fix`, `perf` types only. Scope filter:
- Core changelog: scope `core` OR `pkg`
- Triage changelog: scope `triage` OR `pkg`

A `feat(pkg): ...` commit appears in BOTH changelogs the next time each binary releases — `pkg/` is shared framework code, accurate.

The triage changelog is currently absent from the GitHub Release body because the `gh release create` shim doesn't pass a `--notes-file`. Goreleaser still writes the changelog into `dist/triage/CHANGELOG.md`; reading it from there is left as a follow-up.

## Adding a second first-party plugin

See `RELEASE.md` "What changes mid-cycle" section. Five touchpoints: `registry.go::builtin`, `.commitlintrc.yml` scope-enum (also `.github/workflows/commit-lint.yml`), new `.goreleaser.<plugin>.yaml` (including `release: { disable: true }` and the matching `dist:`/`builds.main`/`ldflags`/`archives.name_template`), new Make target (also add to `.PHONY`), README/`RELEASE.md` updates. Hand-rolled per plugin — Pro `monorepo.tag_prefix` would have made it generic.

## Quick reference

| Task | Command |
|---|---|
| Dry-run both configs locally | `make release-snapshot` |
| Cut core release | `git tag vX.Y.Z && git push origin vX.Y.Z && make release-core` |
| Cut triage release | `git tag plugins/triage/vX.Y.Z && git push origin plugins/triage/vX.Y.Z && make release-triage` |
| Verify brew install | `brew update && brew install dmastrorillo/tap/tai && tai --version` |
| Verify go install | `go install github.com/dmastrorillo/tai/core/cmd/tai@vX.Y.Z` |
| Verify plugin install | `tai plugins install triage --version vX.Y.Z` |
