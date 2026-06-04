## Context

The `pivot-to-ai-as-code` proposal landed the monorepo (`core/`, `plugins/`, `pkg/`), the plugin host, and the update-banner — and deferred the release pipeline. Today nothing TAI publishes is reachable from `brew`, `go install` against a tagged version, or the `tai plugins triage install` flow (which assumes a GitHub Release exists with the right asset names).

Two structural facts shape every decision below:

1. **One Go module.** `go.mod` lives at the repo root with module path `github.com/dmastrorillo/tai`. There is no `core/go.mod` or `plugins/triage/go.mod`. Splitting the module is a much larger refactor that buys little.
2. **Go's module-version rule.** A version tag `<subdir>/vX.Y.Z` only resolves as a valid module version when `<subdir>` contains its own `go.mod`. So under our single-module layout, only bare `vX.Y.Z` tags at the repo root are valid versions for the module.

Constraint (2) is the hidden fence the rest of the design grew against — see Decision D1.

The plugin host has already pinned one downstream contract: `core/internal/plugins.AssetFilename` expects `tai-plugin-<name>-<os>-<arch>.tar.gz` to be present on the GitHub Releases of the source repo. Whatever GoReleaser produces MUST match that filename byte-for-byte.

This design replaces what would normally be a separate ADR. Future readers looking for "why this shape?" should find their answers here, not in `docs/adr/`.

## Goals / Non-Goals

**Goals:**

- A maintainer can cut a `tai` release from their laptop in three commands: tag, push, `make release-core`.
- A maintainer can cut a `triage` plugin release independently with the analogous three commands.
- A non-Go-developer on macOS can install TAI with one line: `brew install dmastrorillo/tap/tai`.
- A Go developer on any platform can install TAI with `go install github.com/dmastrorillo/tai/core/cmd/tai@latest`.
- The plugin host's `tai plugins triage install/update` flow works against real GitHub Releases produced by this pipeline, with no further changes required.
- Pre-releases are publishable for dogfooding without nagging stable users.
- Every change to the pipeline is debuggable end-to-end on a laptop — CI is a port, not a prerequisite.

**Non-Goals:**

- GitHub Actions CI release workflow. Local-first first; CI port is a separate proposal once the local flow is proven.
- A `tai-beta` brew formula or any other pre-release channel beyond explicit `--version` opt-in.
- Linux package managers (`.deb`, `.rpm`, `apk`, AUR). GoReleaser can do this later via `nfpms:`/`aurs:`; no demand today.
- `homebrew/core` upstreaming. Notability bar not met; self-tap is fine for v0.x.
- Tag GPG signing.
- A v1.0 SemVer compatibility promise. v0.x stays "anything goes between minor bumps."
- Multi-module split (per-binary `go.mod`). Considered and rejected — see D1.

## Decisions

### D1 — Tag scheme: bare `vX.Y.Z` for core, `plugins/<name>/vX.Y.Z` for plugins

The asymmetry is forced by Go's module-version rule (Context, fact 2). For `go install github.com/dmastrorillo/tai/core/cmd/tai@vX.Y.Z` to work, the tag governing that path must be `vX.Y.Z` at the repo root. Prefixing core tags (`core/vX.Y.Z`) would break `go install` entirely.

For plugins, no such constraint exists — they are not installable via `go install` (the plugin host requires binaries to live under `<TAI_DATA_DIR>/plugins/<name>/`, not `$GOPATH/bin`). So plugins can and should use a prefix that disambiguates them from core in the same Releases page. `plugins/<name>/` is plural to match the existing `plugins/` directory, the `tai plugins` verb, and the `plugins.yml` filename — the codebase already uses the plural form for this concept.

**Alternatives considered:**

- **Single shared tag stream `vX.Y.Z` releases everything.** Simpler infra (one GoReleaser run). Rejected because CLAUDE.md (and reality, once a second first-party plugin exists) wants independent cadences. With one stream, every release bumps every binary regardless of whether it changed.
- **Prefixed tags for core too (`core/vX.Y.Z`).** Symmetric, prettier, but breaks `go install`. Could be salvaged by splitting the module — see D2.
- **Split the module per binary.** `core/go.mod` (path `github.com/dmastrorillo/tai/core`), `plugins/triage/go.mod`. Then `core/v0.6.0` becomes a valid submodule version. Considered and rejected: the refactor cost (every cross-tree import gets versioned, `go.work` for local development, `pkg/` becomes a third module, every BDD test harness recompiles) is much larger than the tag-asymmetry it cures. Asymmetry is uncomfortable but matches the asymmetry already in the repo: there is one Go module with many binaries inside it.

### D2 — Two GoReleaser configs, not one

`.goreleaser.core.yaml` and `.goreleaser.triage.yaml`. Each carries `monorepo.tag_prefix:` to scope which tags trigger it (core leaves it empty; triage uses `plugins/triage/`).

The structural reason: GoReleaser ties one config to one release. If core and triage shared a config and a tag, every "triage hotfix" would also re-tag core, and every changelog would mix both binaries' commits.

Splitting also keeps the brew block (which only applies to core) and the archive name template (which differs per binary) cleanly separated. A single config with conditionals would be a bigger maintenance hazard than two parallel files.

**Alternative:** one `.goreleaser.yaml` with multiple `builds` entries. Easier to author initially, harder to evolve once the configs diverge — and they will (brew applies to core only; archive naming differs; in the future, signing keys may differ).

### D3 — Pre-releases supported, opt-in only

GoReleaser auto-marks any tag with a SemVer pre-release suffix (`v0.6.0-rc.1`, `v0.6.0-beta.2`) as `prerelease: true` on the GitHub Release page.

- **Banner ignores prereleases.** A user on stable should never see "v0.6.0-rc.1 available" as a nag. The new prefix-aware lookup (D5) filters `prerelease: true` explicitly; the `/releases/latest` endpoint we still use for core already excludes them.
- **`tai plugins <name> install/update` without `--version` ignores prereleases.** Same filter as the banner — "latest" means "latest stable."
- **Explicit opt-in works:** `tai plugins <name> install --version v0.5.0-beta.2`, `go install ...@v0.6.0-rc.1`, direct download from the (visible) Release page.
- **Brew formula serves stable only.** GoReleaser's `brews:` block doesn't publish for pre-releases by default; we keep that default. A `tai-beta` formula is YAGNI until someone asks.

This is what `gh`, `helm`, `kubectl`, and `goreleaser` itself do. The contract is recognisable.

### D4 — Self-hosted brew tap, separate repo

`dmastrorillo/homebrew-tap`. GoReleaser's `brews:` block on `.goreleaser.core.yaml` writes the formula and pushes it to that repo on every core release, using `HOMEBREW_TAP_GITHUB_TOKEN` (a PAT with write scope to the tap repo).

**Alternatives:**

- **homebrew/core (upstream).** Bare `brew install tai`. Rejected: notability bar (stars/watchers, "established" project), every release becomes a PR against a third-party repo reviewed by maintainers. Real friction. Defer.
- **Tap in this repo (Formula/ directory at root).** Avoids a second repo. Rejected because `brew tap dmastrorillo/tai` would conflict with the natural impulse to call the source-of-truth repo `tai`, and mixing release artifacts with source is a code-smell.

Plugins do not get brew formulae. Even a hypothetical `tai-plugin-triage` formula would install the binary to `/opt/homebrew/bin/triage`, but the plugin host discovers plugins by reading `<TAI_DATA_DIR>/plugins/<name>/<name>` — wrong path, invisible to `tai`. Plugin installation is exclusively `tai plugins <name> install`. This constraint is added to CLAUDE.md so a future contributor doesn't try to "helpfully" add a triage formula.

### D5 — Banner + plugin-host fetch: list-and-filter for plugin "latest" lookups

The current `/releases/latest` endpoint returns the most recent non-prerelease release REGARDLESS of tag prefix. Under D1's prefixed plugin tags, "what's the latest triage release?" can return a core `vX.Y.Z` release if it was published more recently than any `plugins/triage/v*` release.

**Algorithm for plugin "latest" lookups (banner plugin-rows; `tai plugins <name> install/update` without `--version`):**

1. `GET /repos/{repo}/releases?per_page=100`.
2. Filter entries whose `tag_name` starts with the plugin's prefix (`plugins/<name>/`).
3. Drop entries where `prerelease: true`.
4. Strip the prefix, parse the remainder as semver, drop entries that don't parse.
5. Return the maximum semver.
6. If the list is empty: return "no release" (banner skips the row; install fails with `PLUGIN_FETCH_FAILED` quoting the prefix it searched for).

**Core "latest" lookup stays unchanged.** `/releases/latest` is correct for core — core tags carry no prefix and the endpoint already filters prereleases. Two code paths is the right cost for one bare-tag stream and one prefixed-tag stream.

`per_page=100` is a deliberate ceiling. We don't expect more than 100 releases to be relevant for this filter for a long time; if we ever do, the lookup degrades to "miss the latest" rather than crash. Worth a comment, not pagination today.

**Alternative considered:** keep `/releases/latest` and rely on chronological order. Rejected — it produces correct answers ONLY when core and the plugin happen to release in lockstep, which is exactly what D1's independent prefixed streams reject.

### D6 — Changelog: auto-generated, scope-filtered, `pkg` in both

GoReleaser auto-generates a changelog from git commits between the current tag and the previous matching-prefix tag. We filter by Conventional Commits scope:

| Scope     | Core changelog | Triage changelog |
|-----------|---------------|------------------|
| `core`    | yes           | no               |
| `triage`  | no            | yes              |
| `pkg`     | yes           | yes              |
| `openspec`| no            | no               |
| `ci`      | no            | no               |

Only `feat`, `fix`, `perf` types appear; `docs`/`chore`/`style`/`test`/`refactor`/`build` are excluded as noise.

The `pkg`-in-both is the only subtle bit: shared framework code genuinely changes behaviour in BOTH binaries, so it belongs in BOTH changelogs the next time each releases. This produces real duplication across the two changelogs, which is accurate, not a bug.

This filtering only works if commit subjects follow the scope convention, which is exactly what D7 enforces.

### D7 — Re-enable commit-lint CI

`.github/workflows/commit-lint.yml` and `.commitlintrc.yml` already exist and were carefully scoped during the pivot. The workflow is gated to `on: workflow_dispatch` (effectively off). Flip to `on: pull_request` and the scope discipline that D6 depends on is enforced automatically on every PR.

The existing scope enum (`core`, `pkg`, `triage`, `openspec`, `ci`) already covers every change this proposal introduces — Makefile edits, GoReleaser configs, and the version-string LD-flag injection all live under `ci`. No new scopes are needed.

### D8 — Local-first trigger, CI as a follow-up

A maintainer runs:

```
git tag <tag>
git push origin <tag>
goreleaser release --config <config> --clean
```

…with `GITHUB_TOKEN` (releases + tap repo write) in the env. The Makefile wraps the third step:

- `make release-snapshot` — both configs with `--snapshot --clean --skip=publish,announce`. Produces `dist/` archives without publishing. Run before tagging to validate.
- `make release-core` — `goreleaser release --config .goreleaser.core.yaml --clean`.
- `make release-triage` — `goreleaser release --config .goreleaser.triage.yaml --clean`.

Why local-first: every CI failure on a release pipeline is debugged on a laptop anyway. Building the laptop flow first means the CI port is a near-trivial rewrite (`on: push: tags: 'v*'` for core, `'plugins/*/v*'` for triage, same `goreleaser release --clean` command, secrets stored at repo level). Building CI first means every iteration is a 3-minute round-trip through Actions.

### D9 — Versioning policy: v0.x is "anything goes"

Standard Go-module convention. Minor bumps within v0.x may include breaking changes. There is no SemVer compatibility promise until someone declares v1.0. Core and triage version independently — triage at v0.5.0 while core is at v0.6.0 is normal and expected (and is exactly the state we'll be in after the first release of each).

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Tag asymmetry (bare core vs prefixed plugin) confuses contributors. | Documented in CONTRIBUTING.md ("Cutting a release") with both examples side-by-side; Makefile targets pre-bake the right command per binary. |
| The two-config GoReleaser setup drifts (e.g. archive name changes in one but not the other). | `make release-snapshot` runs both. A BDD case in `pkg/test-cases.md` pins the triage archive filename against `core/internal/plugins.AssetFilename`'s expectation — drift causes a red test, not a runtime fetch failure six months later. |
| Brew tap PAT leaks. | Stored only in the maintainer's local shell env for the local-first phase. When CI is added, scoped to write-only on the tap repo. |
| `per_page=100` ceiling silently drops the true latest plugin release. | 100 is generous for any single binary's release stream within this repo for years. Documented at the lookup site. Pagination is a future change with a one-line refactor (loop until `Link: rel="next"` is absent). |
| `monorepo.tag_prefix` in GoReleaser is only available in v1.13+. | Pin the GoReleaser version in the Makefile invocation comment and in CONTRIBUTING; surface a clear error if the installed binary is too old. |
| A `feat(pkg): ...` commit between a core release and a triage release appears in BOTH changelogs — looks like duplication. | Accurate, not a bug. Both binaries' behaviour really did change. Documented in CONTRIBUTING. |
| Re-enabling commit-lint blocks PRs whose commit messages don't conform. | A grace PR (separate from the release-cycle one) re-enables it; any blocked author gets a clear error pointing at `.commitlintrc.yml`. |
| The tap repo doesn't exist when someone runs the first `make release-core`. | Captured as a tasks.md item (one-time manual setup). GoReleaser will fail loudly with a clear error if the tap repo is missing — not silent. |
| Brew formula assumes a Mac/Linux user; Windows-on-WSL users get a worse path. | Acceptable. Windows users have `go install` and direct binary download. |
| `CGO_ENABLED=0` cross-compile depends on `modernc.org/sqlite` staying pure-Go. | If a future PR adds a CGO dependency, the snapshot build breaks immediately and visibly. Worth a note in CONTRIBUTING ("don't add CGO deps without a release-cycle conversation"). |
