## 1. Re-enable commit-lint enforcement

- [x] 1.1 Verify the existing `scope-enum` in `.commitlintrc.yml` covers `core`, `pkg`, `triage`, `openspec`, `ci`. Confirm no scope is missing for the release-cycle work.
- [x] 1.2 Edit `.github/workflows/commit-lint.yml` — replace `on: workflow_dispatch` with `on: pull_request` and the standard `types: [opened, edited, reopened, synchronize]` filter (the file already documents this swap in its own header comment).
- [ ] 1.3 Open a deliberately-bad-commit-message draft PR against this branch to verify the workflow blocks merge; close without merging once confirmed.

## 2. One-time external setup (capture, do not automate)

- [ ] 2.1 Create the public GitHub repo `dmastrorillo/homebrew-tap` (empty — first GoReleaser run will populate it). Add a one-line README explaining it is auto-managed by GoReleaser.
- [ ] 2.2 Generate a fine-grained PAT with `Contents: Write` scope limited to `dmastrorillo/homebrew-tap`. Document under what env var the maintainer should expose it locally (`HOMEBREW_TAP_GITHUB_TOKEN`).
- [ ] 2.3 Pin a minimum GoReleaser version (>= v1.13 for `monorepo.tag_prefix`) in a `RELEASE.md` note alongside install instructions for macOS (`brew install goreleaser/tap/goreleaser`) and Linux.

## 3. GoReleaser configurations

- [x] 3.1 Author `.goreleaser.core.yaml`. Confirm via inspection: empty `monorepo.tag_prefix`; single `builds` entry for `./core/cmd/tai` with `env: [CGO_ENABLED=0]`; `goos: [linux, darwin, windows]`, `goarch: [amd64, arm64]`; `ldflags: -X github.com/dmastrorillo/tai/core/internal/version.String={{ .Version }}`; archives `tai_{{ .Os }}_{{ .Arch }}` (default `.tar.gz`, `.zip` for Windows); `brews:` block targeting `dmastrorillo/homebrew-tap` with `token: {{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}`; `changelog` block with type filter (`feat`, `fix`, `perf`) and `filters.exclude` for the non-core scopes per the `release-cycle` capability spec.
- [x] 3.2 Author `.goreleaser.triage.yaml`. Confirm: `monorepo.tag_prefix: plugins/triage/`; single `builds` entry for `./plugins/triage/cmd/triage` with same env/goos/goarch; `ldflags: -X github.com/dmastrorillo/tai/plugins/triage/internal/version.String={{ .Version }}`; archive name template that produces `tai-plugin-triage-{{ .Os }}-{{ .Arch }}.tar.gz` (cross-check against `core/internal/plugins.AssetFilename("triage", os, arch)` — must be byte-identical); NO `brews:` block; `changelog` block with `triage|pkg` scope filter.
- [ ] 3.3 Run `goreleaser release --config .goreleaser.core.yaml --snapshot --clean --skip=publish,announce`. Inspect `dist/` for `tai_darwin_arm64.tar.gz`, `tai_linux_amd64.tar.gz`, etc.; extract one and run `./tai --version` to confirm the linker injected `0.0.0-next` or similar (snapshot version, not `dev`).
- [ ] 3.4 Run `goreleaser release --config .goreleaser.triage.yaml --snapshot --clean --skip=publish,announce`. Inspect `dist/` for `tai-plugin-triage-darwin-arm64.tar.gz` etc.; extract one and confirm the version string matches the snapshot tag.
- [x] 3.5 Write a small Go test (or shell check) that asserts `core/internal/plugins.AssetFilename("triage", "darwin", "arm64") == "tai-plugin-triage-darwin-arm64.tar.gz"` is the exact filename emitted by step 3.4. This pins the contract — drift causes a red test, not a runtime fetch failure six months later. Landed as TC-REL-001 in `core/test-cases.md` + `core/internal/plugins/fetch_test.go::TestAssetFilename_TCREL001_matches_goreleaser_archive_name`.

## 4. Makefile targets

- [x] 4.1 Add `release-snapshot` target to `Makefile`. Body: runs both `.goreleaser.*.yaml` configs with `--snapshot --clean --skip=publish,announce`. Adds the target to the `help` block.
- [x] 4.2 Add `release-core` target. Body: `goreleaser release --config .goreleaser.core.yaml --clean`. Adds to `help`. No env-var precondition in the target — GoReleaser surfaces missing tokens itself.
- [x] 4.3 Add `release-triage` target. Body: `goreleaser release --config .goreleaser.triage.yaml --clean`. Adds to `help`.

## 5. Banner + plugin-host fetch refactor (TDD per CLAUDE.md)

- [x] 5.1 Add BDD cases for the prefix-aware lookup algorithm under a new `TC-REL-*` category. (Re-targeted from `pkg/test-cases.md` to `core/test-cases.md`: the fetcher lives in `core/internal/plugins`, not `pkg/`, so behavioural cases for it belong in core's file. The pkg/test-cases.md file only covers the public `pkg/` stability contract, and `pkg/` is unchanged by this proposal.) Cases TC-REL-002..005 cover prefix filtering, prerelease exclusion, max-semver selection, malformed-tag tolerance. ToC entry added.
- [x] 5.2 Add BDD cases to `core/test-cases.md` for the banner's plugin-row behaviour under the same `REL` category: TC-REL-006 (banner uses prefix-aware lookup, no cross-contamination), TC-REL-007 (banner suppresses pre-release plugin tags), TC-REL-008 (banner core-row continues using `/releases/latest`).
- [x] 5.3 Write failing tests that name the new TC-IDs (one test per case). Run; confirm red for the right reason. Confirmed: both `core/internal/plugins` and `core/internal/sync` fail to compile on `LatestPrefixedTag` / `LatestPrefixedTagForTesting` undefined.
- [x] 5.4 Implement the shared algorithm. Landed as `core/internal/plugins.LatestPrefixedTag` in `core/internal/plugins/latest.go` — a package-level function (not a method on `*HTTPFetcher`) so both the fetcher and the banner package can call it directly with their own HTTP clients. Returns `(string, error)`; `("", nil)` is the "no matching stable release" sentinel.
- [x] 5.5 Update `core/internal/plugins/fetch.go` — when `src.Version` is empty, call `LatestPrefixedTag` with prefix from `PluginTagPrefix(name, src)` (new helper in `core/internal/plugins/prefix.go`: returns `plugins/<name>/` for first-party monorepo plugins, `""` for third-party). Removed the `/releases/latest` fallback in `lookupAsset` — empty tag is now an explicit programmer error there.
- [x] 5.6 Update `core/internal/sync/banner.go` — the plugin loop in `extendPollWithBannerLayers` now calls `callLatestPrefixedTag` with the prefix from `plugins.PluginTagPrefix`. TAI core's row still calls the legacy `callLatestTag` (kept).
- [x] 5.7 Added `LatestPrefixedTagFn` + `latestPrefixedTag` package-level var + `callLatestPrefixedTag` accessor + `LatestPrefixedTagForTesting(t, fn)` helper, mirroring the existing `LatestTagForTesting` pattern. `stubPrefixed` test helper added alongside `stubLatest`.
- [x] 5.8 `go test ./... -count=1` and `go test -race -count=1 ./...` both pass: 452 tests across 34 packages.

## 6. CLAUDE.md amendments

- [x] 6.1 Edit the "Conventions" section: rewrite the bullet about linker-injectable build-metadata variables. Drop the "Each binary owns its own version package — core and each plugin ship on independent release lifecycles" clause. Replace with: "Each binary owns its own version package so the linker can inject distinct values; first-party binaries in this repo ship together from prefixed and bare tags managed by the `release-cycle` capability, third-party plugins ship on their own cadence from their own repos."
- [x] 6.2 Edit the "Plugin host" section: add a one-paragraph constraint titled "Plugins are NOT distributable via Homebrew." Body: brew installs to `/opt/homebrew/bin/`; the plugin host requires binaries at `<TAI_DATA_DIR>/plugins/<name>/<name>`; the only supported plugin install path is `tai plugins <name> install`; do not author brew formulae for first-party plugins.
- [x] 6.3 Edit `core/internal/version/version.go` package comment: remove the line "Each binary in this module owns its own version package — the triage plugin's binary has an independent copy under plugins/triage/internal/version so its release lifecycle stays decoupled from core." Replace with: "Each binary in this module has its own version package so the linker can inject distinct values; first-party binaries share commits and release via the prefix-aware tag scheme owned by the `release-cycle` capability."
- [x] 6.4 Edit `plugins/triage/internal/version/version.go` package comment: equivalent rewrite to 6.3 — drop "independent of the core tai binary's version under core/internal/version; the two binaries ship from the same repo but on independent release cadences after the pivot."

## 7. Documentation

- [x] 7.1 Update README.md "Install" section with three subsections: Homebrew (`brew install dmastrorillo/tap/tai`), `go install` (`go install github.com/dmastrorillo/tai/core/cmd/tai@latest`), and Direct binary (link to Releases page). Mention plugins are installed via `tai plugins <name> install` only.
- [x] 7.2 Add `RELEASE.md` at repo root (or extend CONTRIBUTING.md) documenting: tag conventions (bare for core, `plugins/<name>/` for plugins); the three Makefile targets and required env vars; the minimum GoReleaser version; the one-time tap-repo setup; the future CI port.

## 8. Pre-validation

- [x] 8.1 Run `openspec validate release-cycle --strict`. Passes: "Change 'release-cycle' is valid".
- [x] 8.2 Run `make check` (the existing pre-merge sweep — `fmt-check`, `vet`, `test`, `test-race`). Clean; 452 tests across 34 packages, race-clean.
- [ ] 8.3 Run `make release-snapshot` end-to-end. Confirm both configs produce expected `dist/` contents with the correct archive names. **Pending maintainer**: `goreleaser` not installed on dev machine in this session; install via `brew install goreleaser/tap/goreleaser` then run `make release-snapshot`.

## 9. First real release (separate session, not part of this proposal's PR)

- [ ] 9.1 Decide the first core version number (likely `v0.1.0` or `v0.6.0` aligning with the current pivot ordinal — confirm with the maintainer).
- [ ] 9.2 Decide the first triage version number (likely `v0.1.0`).
- [ ] 9.3 Run `git tag <vX.Y.Z> && git push origin <vX.Y.Z> && make release-core`. Verify the GitHub Release page, the brew tap commit, and `brew install dmastrorillo/tap/tai` end-to-end.
- [ ] 9.4 Run `git tag plugins/triage/<vX.Y.Z> && git push origin plugins/triage/<vX.Y.Z> && make release-triage`. Verify the GitHub Release page and `tai plugins triage install` against the new release end-to-end.

## 10. Archive

- [ ] 10.1 After all of sections 1–8 are merged to main (and section 9 has produced the first real release), run `openspec archive release-cycle` and commit the resulting move under `openspec/changes/archive/`.
