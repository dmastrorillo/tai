# release-cycle Specification

## Purpose
TBD - created by archiving change release-cycle. Update Purpose after archive.
## Requirements
### Requirement: Tag scheme

The system SHALL use two distinct git tag patterns for releases from this repository:

- **Core binary** (`tai`): bare semantic-version tags at the repo root in the form `vX.Y.Z` (with optional SemVer pre-release suffix `vX.Y.Z-<prerelease>`).
- **First-party plugin binaries**: prefixed tags in the form `plugins/<name>/vX.Y.Z` (with optional pre-release suffix). The prefix MUST match the plugin's directory name under `plugins/` and the plugin's CLI verb registered in `core/internal/verbs.Reserved()` adjacency rules.

The bare-vs-prefixed asymmetry is forced by Go's module-version rule: this repository contains a single `go.mod` at the root with module path `github.com/dmastrorillo/tai`, so only bare `vX.Y.Z` root tags are valid versions for `go install github.com/dmastrorillo/tai/...@vX.Y.Z`. Plugins, which are not installable via `go install`, MAY use prefixed tags freely.

#### Scenario: Core release tag

- **WHEN** the maintainer tags the repo head with `v0.6.0` and pushes the tag
- **THEN** that tag is a valid Go module version for `github.com/dmastrorillo/tai`
- **AND** `go install github.com/dmastrorillo/tai/core/cmd/tai@v0.6.0` resolves successfully

#### Scenario: Plugin release tag

- **WHEN** the maintainer tags the repo head with `plugins/triage/v0.5.0` and pushes the tag
- **THEN** the tag is recognized by `.goreleaser.triage.yaml` (matches `monorepo.tag_prefix: plugins/triage/`)
- **AND** it is NOT a valid Go module version (Go's submodule-tag rule does not apply because no `plugins/triage/go.mod` exists)

#### Scenario: Pre-release suffix on either stream

- **WHEN** the maintainer tags `v0.6.0-rc.1` (core) or `plugins/triage/v0.5.0-beta.2` (triage)
- **THEN** the resulting GitHub Release is flagged `prerelease: true`
- **AND** the tag is ignored by every "latest" lookup defined in this capability

### Requirement: GoReleaser configuration layout

The repository SHALL provide two GoReleaser configuration files at the repo root:

- `.goreleaser.core.yaml` — builds and releases the `tai` core binary. Triggered by bare `vX.Y.Z` tags at the repo root (GoReleaser v2's default tag matching). Its `archives` block MUST produce `tai_<os>_<arch>.tar.gz` (Windows: `.zip`). Its `homebrew_casks:` block MUST target the `dmastrorillo/homebrew-tap` repository. Its `builds.ldflags` MUST inject `-X github.com/dmastrorillo/tai/core/internal/version.String=v{{ .Version }}`.
- `.goreleaser.triage.yaml` — builds the `triage` plugin binary; release publishing is delegated to `gh` CLI (see "Plugin release tag-prefix handling" below). Its `archives` block MUST produce `tai-plugin-triage-<os>-<arch>.tar.gz` (byte-identical to `core/internal/plugins.AssetFilename("triage", os, arch)`). Its `builds.ldflags` MUST inject `-X github.com/dmastrorillo/tai/plugins/triage/internal/version.String=v{{ .Version }}`. It MUST set `release: { disable: true }` and MUST NOT define a `homebrew_casks:` block.

Both configurations SHALL build for the matrix `{linux, darwin, windows} × {amd64, arm64}` with `CGO_ENABLED=0`.

Adding a future first-party plugin SHALL add a third `.goreleaser.<plugin>.yaml` following the same shape — never extend `.goreleaser.core.yaml`.

#### Scenario: Triage archive name matches plugin-host expectation

- **WHEN** `.goreleaser.triage.yaml` is invoked via `goreleaser release --snapshot --clean`
- **THEN** the resulting `dist/` directory contains files named `tai-plugin-triage-darwin-arm64.tar.gz`, `tai-plugin-triage-linux-amd64.tar.gz`, etc.
- **AND** for every `(os, arch)` pair, the filename equals the return value of `core/internal/plugins.AssetFilename("triage", os, arch)`

#### Scenario: Linker injects core version

- **WHEN** `.goreleaser.core.yaml` is invoked with the working tree at tag `v0.6.0`
- **THEN** every produced `tai` binary, when run with `--version`, prints `v0.6.0` (not `dev`)

#### Scenario: Linker injects triage version

- **WHEN** `.goreleaser.triage.yaml` is invoked with the working tree at tag `plugins/triage/v0.5.0`
- **THEN** every produced `triage` binary's install-summary version banner reports `v0.5.0` (not `dev`)

### Requirement: Prefix-aware latest release lookup

For any "latest version" lookup against a release stream that uses prefixed tags (i.e. plugin streams under `plugins/<name>/`), the system SHALL use the following algorithm:

1. `GET /repos/{org}/{repo}/releases?per_page=100` against the source repo.
2. Filter responses where `tag_name` begins with `plugins/<name>/`.
3. Drop responses where `prerelease` is `true`.
4. Strip the `plugins/<name>/` prefix from `tag_name`; parse the remainder as semantic version. Drop entries that fail to parse.
5. Select the entry with the maximum semantic version.
6. If the filtered list is empty, return a "no release" sentinel — callers MUST treat this as "no update available," not as an error.

This algorithm SHALL be the sole mechanism by which:

- The update banner determines the latest available version of an installed plugin.
- `tai plugins <name> install` and `tai plugins <name> update` determine the latest version when `--version` is omitted.

Lookups against the core release stream (bare `vX.Y.Z` tags) SHALL continue to use the GitHub `/repos/{org}/{repo}/releases/latest` endpoint, which excludes pre-releases natively and is correct for an unprefixed tag stream.

#### Scenario: Mixed core and plugin releases on the same repo

- **GIVEN** the source repo `dmastrorillo/tai` has the following published releases (newest first by `published_at`): `v0.6.1` (core), `plugins/triage/v0.5.0` (triage), `v0.6.0` (core), `plugins/triage/v0.4.0` (triage)
- **WHEN** the system asks "what is the latest triage release?"
- **THEN** the algorithm returns `v0.5.0` (the highest semver among entries whose `tag_name` starts with `plugins/triage/`)
- **AND** the `v0.6.1` core release is NOT returned despite being chronologically newer

#### Scenario: Pre-release plugin tag filtered out

- **GIVEN** the source repo's most recent triage release is `plugins/triage/v0.5.0-rc.1` (marked `prerelease: true`)
- **AND** the previous stable triage release is `plugins/triage/v0.4.0`
- **WHEN** the system asks "what is the latest stable triage release?"
- **THEN** the algorithm returns `v0.4.0`

#### Scenario: No matching plugin releases

- **GIVEN** the source repo has only core releases — no `plugins/triage/*` tags exist
- **WHEN** the system asks "what is the latest triage release?"
- **THEN** the algorithm returns the "no release" sentinel
- **AND** the caller treats this as "no update available," not as an error

#### Scenario: Malformed plugin tag tolerated

- **GIVEN** the source repo has `plugins/triage/v0.5.0` and `plugins/triage/oops-not-a-version`
- **WHEN** the system asks "what is the latest triage release?"
- **THEN** the algorithm returns `v0.5.0`
- **AND** the malformed tag is silently dropped (no error, no warning)

### Requirement: Pre-release semantics

Pre-release tags (those with a SemVer pre-release suffix — `v0.6.0-rc.1`, `plugins/triage/v0.5.0-beta.2`) SHALL be supported but opt-in only.

GoReleaser SHALL flag any tag with a pre-release suffix as `prerelease: true` on the resulting GitHub Release. The system SHALL NOT surface such releases through any "latest" lookup (banner row, `tai plugins <name> install/update` without `--version`).

A user opts into a pre-release through one of three explicit paths:

1. `tai plugins <name> install --version vX.Y.Z-rc.N` (or the analogous `update` with `--version`).
2. `go install github.com/dmastrorillo/tai/core/cmd/tai@vX.Y.Z-rc.N`.
3. Direct download of the pre-release archive from the GitHub Releases page.

The Homebrew formula SHALL serve stable releases only.

#### Scenario: Banner skips pre-release plugin tag

- **GIVEN** the most recent triage release is `plugins/triage/v0.5.0-rc.1` (pre-release)
- **AND** the installed triage version is `v0.4.0`
- **WHEN** the daily update banner is evaluated
- **THEN** no banner row appears for triage (the pre-release is excluded; no stable upgrade is available)

#### Scenario: Explicit pre-release install succeeds

- **WHEN** the user runs `tai plugins triage install --version v0.5.0-rc.1`
- **AND** the source repo has a release at tag `plugins/triage/v0.5.0-rc.1`
- **THEN** the install proceeds normally
- **AND** the recorded version in `<TAI_DATA_DIR>/state/plugins.json` is `v0.5.0-rc.1`

#### Scenario: Brew formula skipped for pre-release

- **WHEN** the maintainer publishes core release `v0.6.0-rc.1`
- **THEN** the GoReleaser `brews:` block does NOT push a formula update to `dmastrorillo/homebrew-tap`

### Requirement: Homebrew distribution

The system SHALL distribute the `tai` core binary via a self-hosted Homebrew tap at the repository `dmastrorillo/homebrew-tap`. On every core release (non-pre-release), GoReleaser SHALL write the updated `tai.rb` formula and push it to that tap.

The install command for end users SHALL be `brew install dmastrorillo/tap/tai`.

Plugin binaries SHALL NOT be distributed via Homebrew. The plugin host requires plugin binaries to reside at `<TAI_DATA_DIR>/plugins/<name>/<name>`; binaries installed by Homebrew to `/opt/homebrew/bin/` are not discoverable by the plugin host. Plugin installation is exclusively via `tai plugins <name> install`.

#### Scenario: Tap formula updated on core release

- **WHEN** the maintainer runs `make release-core` against tag `v0.6.0` with a valid `HOMEBREW_TAP_GITHUB_TOKEN`
- **THEN** a commit is pushed to `dmastrorillo/homebrew-tap` updating `Casks/tai.rb` to point at the `v0.6.0` archives
- **AND** `brew install dmastrorillo/tap/tai` installs version `v0.6.0`

#### Scenario: Brew install reports correct version

- **WHEN** a user installs the formula and runs `tai --version`
- **THEN** the printed version matches the formula's release tag (not `dev`)

### Requirement: `go install` paths

The system SHALL support `go install github.com/dmastrorillo/tai/core/cmd/tai@latest` and `go install github.com/dmastrorillo/tai/core/cmd/tai@vX.Y.Z` for the core binary, against any pushed bare `vX.Y.Z` tag.

`go install` SHALL NOT be a supported install path for plugin binaries — plugin discovery requires the binary to live under `<TAI_DATA_DIR>/plugins/<name>/`.

#### Scenario: go install at @latest

- **WHEN** the user runs `go install github.com/dmastrorillo/tai/core/cmd/tai@latest` and the repo has a most-recent stable tag `v0.6.0`
- **THEN** `$GOPATH/bin/tai --version` reports `v0.6.0`

#### Scenario: go install at explicit pre-release version

- **WHEN** the user runs `go install github.com/dmastrorillo/tai/core/cmd/tai@v0.6.0-rc.1`
- **AND** the tag `v0.6.0-rc.1` is pushed to the repo
- **THEN** `$GOPATH/bin/tai --version` reports `v0.6.0-rc.1`

### Requirement: Release trigger and Makefile targets

The system SHALL support a local-first release trigger. A maintainer cuts a release by:

1. Tagging the repo head with the correct tag pattern (`vX.Y.Z` for core, `plugins/<name>/vX.Y.Z` for a plugin).
2. Pushing the tag to the origin remote.
3. Running the corresponding Makefile target.

The Makefile SHALL provide three release-related targets:

- `make release-snapshot` — runs both `.goreleaser.*.yaml` configurations with `--snapshot --clean --skip=publish,announce`. Produces archives under `dist/` without publishing or pushing. Has no required env vars. Used to validate config changes locally before tagging.
- `make release-core` — runs `goreleaser release --config .goreleaser.core.yaml --clean`. Requires `GITHUB_TOKEN` (releases scope) and `HOMEBREW_TAP_GITHUB_TOKEN` (write scope on the tap repo) in the env. GoReleaser will refuse to run if the working tree is not at a tag or has uncommitted changes — this is the expected gate.
- `make release-triage` — two-step plugin-release publishing. (1) Extracts the bare semver from the current `plugins/triage/vX.Y.Z` tag and runs `GORELEASER_CURRENT_TAG=<bare> goreleaser release --config .goreleaser.triage.yaml --clean` to build archives under `dist/triage/` (the config disables goreleaser's release-publish step). (2) Runs `gh release create plugins/triage/vX.Y.Z dist/triage/tai-plugin-triage-*.tar.gz dist/triage/checksums.txt --title plugins/triage/vX.Y.Z` (and `--prerelease` when the tag carries a SemVer pre-release suffix). Requires `GITHUB_TOKEN` (used by `gh`) and the `gh` CLI on `$PATH`.

CI/CD execution of these targets via GitHub Actions is explicitly out of scope for this capability and is a separate follow-up.

#### Scenario: release-snapshot validates without publishing

- **WHEN** the maintainer runs `make release-snapshot` on a dirty working tree
- **THEN** archives are produced under `dist/core/` and `dist/triage/`
- **AND** no GitHub Release is created
- **AND** no commit is pushed to `dmastrorillo/homebrew-tap`

#### Scenario: release-core refuses without a tag

- **WHEN** the maintainer runs `make release-core` and HEAD is NOT at a `vX.Y.Z` tag
- **THEN** GoReleaser exits non-zero with a clear error naming the missing tag

### Requirement: Plugin release tag-prefix handling

Because GoReleaser v2's OSS edition lacks `monorepo.tag_prefix` and `release.tag` (both Pro-only), plugin releases SHALL be published via a Make-target shim that:

1. Extracts the bare semantic version (e.g. `v0.5.0`) from the prefixed git tag (e.g. `plugins/triage/v0.5.0`).
2. Sets `GORELEASER_CURRENT_TAG` to the bare semver so GoReleaser computes `.Version` correctly for ldflags injection.
3. Runs GoReleaser with `release: { disable: true }` to BUILD archives only.
4. Invokes `gh release create <full-prefixed-tag> <artifacts>` to publish the GitHub Release at the original prefixed tag.

This pipeline SHALL produce a GitHub Release whose `tag_name` matches the maintainer's pushed tag verbatim (`plugins/triage/vX.Y.Z`) — required because `core/internal/plugins.LatestPrefixedTag` filters releases by tag prefix.

#### Scenario: Bare semver injected into triage binary

- **WHEN** `make release-triage` runs against tag `plugins/triage/v0.5.0`
- **THEN** the resulting `triage` binary, when interrogated for its version, reports `v0.5.0` (not `plugins/triage/v0.5.0`, not `dev`)

#### Scenario: GitHub Release published at prefixed tag

- **WHEN** `make release-triage` runs against tag `plugins/triage/v0.5.0`
- **THEN** a GitHub Release is created on `dmastrorillo/tai` with `tag_name == "plugins/triage/v0.5.0"`
- **AND** the release's assets include `tai-plugin-triage-<os>-<arch>.tar.gz` for every `(os, arch)` in the build matrix
- **AND** the release is NOT marked `prerelease: true` (the tag has no SemVer pre-release suffix)

### Requirement: Conventional Commits scope enforcement

The system SHALL enforce Conventional Commits scopes on every pull request via the `.github/workflows/commit-lint.yml` GitHub Action and the `.commitlintrc.yml` rules file.

The workflow's `on:` trigger MUST be `pull_request` (with `types: [opened, edited, reopened, synchronize]`), not `workflow_dispatch`.

The `scope-enum` in `.commitlintrc.yml` MUST include at least `core`, `pkg`, `triage`, `openspec`, `ci`. Additional scopes MAY be appended in the same PR that introduces a new top-level concern requiring them.

#### Scenario: PR with disallowed scope rejected

- **WHEN** a PR contains a commit with subject `feat(banana): add feature`
- **THEN** the commit-lint workflow reports a failure naming the disallowed scope
- **AND** the PR is blocked from merge

#### Scenario: PR with allowed scope passes

- **WHEN** a PR contains commits with subjects `feat(core): add --verbose flag` and `fix(pkg): correct error code`
- **THEN** the commit-lint workflow reports success

### Requirement: Auto-generated, scope-filtered changelogs

Each GoReleaser run SHALL auto-generate the release's changelog from git commits between the current tag and the previous tag matching the same prefix pattern.

Both configurations SHALL include only commits whose Conventional Commits type is `feat`, `fix`, or `perf`.

`.goreleaser.core.yaml` SHALL include commits whose scope is `core` OR `pkg`. `.goreleaser.triage.yaml` SHALL include commits whose scope is `triage` OR `pkg`. A commit with scope `pkg` MAY appear in both binaries' changelogs the next time each releases — this duplication is intentional and reflects the shared-framework reality.

Commits with scope `openspec`, `ci`, `docs`, `chore`, `style`, `test`, `refactor`, or `build` SHALL be excluded from both changelogs.

#### Scenario: Core changelog includes core + pkg commits

- **GIVEN** the commits between the previous core tag and the new core tag include `feat(core): add X`, `fix(pkg): correct Y`, `chore(ci): bump workflow version`, `feat(triage): unrelated triage change`
- **WHEN** `.goreleaser.core.yaml` runs
- **THEN** the published core release's changelog contains entries for `feat(core): add X` and `fix(pkg): correct Y`
- **AND** does NOT contain entries for `chore(ci): ...` or `feat(triage): ...`

#### Scenario: pkg commit appears in both changelogs

- **GIVEN** a `feat(pkg): add new errcode constant` commit lands on `main`
- **AND** later, both a core release and a triage release are cut (in either order)
- **THEN** the core release's changelog contains an entry for `feat(pkg): add new errcode constant`
- **AND** the triage release's changelog contains the same entry
