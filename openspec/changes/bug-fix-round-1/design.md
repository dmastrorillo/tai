## Context

Eight bugs accumulated against tai v0.1 (core `v0.1.2-...`, triage `plugins/triage/v0.1.0`) during the operator's first week of daily use. They span five capabilities (`plugin-host`, `repo-sync`, `release-cycle`, `update-banner`, `repo-init`) and three layers (core CLI, shared framework, triage plugin). Most are small in isolation, but two of them — the plugin-host help wiring and the third-party trust flow — introduce new wire-contract verbs and new on-disk state that future plugin authors and operators need pinned in a spec, so the cluster is shipped as one umbrella change rather than five drive-by commits.

The user's preference (logged in [[feedback-no-adrs-in-tai]] and [[feedback-openspec-commit-flow]]) is to capture rationale in this `design.md` rather than ADRs, and to land the implementation + spec archive in one commit per capability touched.

Current observable failures driving the change:

- Background `git ls-remote` against a private HTTPS `repo-url` writes `Username for 'https://github.com':` to the terminal and reads stdin while the user is typing into a foreground subcommand. The prompt flashes for a fraction of a second before the process exits, looking like a render glitch to anyone who hasn't read the code.
- `tai --version` on a locally-installed binary prints `v0.1.2-0.20260609004251-72a773c77386` — Go's pseudo-version for a non-tagged `go install`. The release-cycle docs assume `go install ...@vX.Y.Z`; the symlinked-local workflow wasn't anticipated.
- `tai plugins install triage` succeeds but lands no files at `~/.claude/commands/tai-triage/`. The triage v0.1.0 tarball ships only the binary + `LICENSE` (the goreleaser archive `files:` list missed `assets/**/*`), and the host's `ValidateAssetNamespace` walks `skills/`/`agents/` only — a tarball with no `assets/` directory at all passes validation vacuously. Users who then run `tai triage install` (a legacy self-installer left over from the Phase 6 pivot) get files at `~/.claude/commands/tai/` instead of the spec-mandated `tai-triage/`, polluting the namespace owned by `tai install-commands`.
- `tai --help` lists only reserved verbs. Installed plugins are invisible until the user reads the `plugins.json` state file directly.
- Triage's own `assets/commands/{import,triage,verify}.md` reference themselves as `/tai:import`, `/tai:triage`, `/tai:verify` — slash-command names that haven't been correct since the directory routing landed in Phase 4. The real slash commands once routed by the host are `/tai-triage:import` etc.
- `tai plugins install --source <untrusted-host>/<org>/<repo>` runs arbitrary code with no friction. `tai sync` against a source repo whose `plugins.yml` lists third-party plugins does the same auto-installation on every dev's machine.
- `repo init`'s scaffolded README points new operators at `https://docs.tai.sh` — a domain that doesn't exist — and never tells a fresh reader what tai actually is.
- A fresh-install user has no signal pointing at `tai install-commands`. A fresh plugin-install user has no signal pointing at `tai <plugin> help`.

The change must land all eight inside one spec-driven proposal because the plugin-host fixes (`--help-summary`, `PLUGIN_ASSET_MISSING`, third-party prompt) share the wire-contract surface and the data layout (`plugins.json` description field, `state/trust.json`, `state/first-run.json`). Splitting them risks landing a half-state in a stable spec.

## Goals / Non-Goals

**Goals:**

1. Eliminate the credentials-prompt flash without breaking foreground `tai sync` interactive auth.
2. Make installed plugins discoverable from `tai --help` without forcing operators to read `plugins.json`.
3. Detect, at install time, the failure mode that let triage v0.1.0 slip through with no `assets/` directory.
4. Remove the triage self-installer that bypasses host namespacing; route triage assets through `SyncAssetsToTargets` like every other plugin.
5. Add friction proportional to risk for third-party plugin installs (interactive prompt + persistent trust cache); zero friction for first-party plugins listed in the built-in registry.
6. Surface a stable, AI-tool-agnostic onboarding hint on first run AND after every plugin install/update.
7. Stop showing Go pseudo-version strings to users; collapse them to `dev` so symlinked-local builds are visually distinct from real releases.
8. Update the scaffolded `tai repo init` README to be correct (no `docs.tai.sh`) and self-explanatory (one-paragraph intro + GitHub backlink).
9. Update CLAUDE.md, RELEASE.md, and the `tai-release` skill so future contributors understand the new rules.

**Non-Goals:**

1. A general "permissions" system for plugins (network gating, filesystem sandboxing, etc.). Third-party plugins run as the user; the trust prompt is purely an "are you sure?" gate.
2. A plugin-manifest file (`plugin.yaml`, `manifest.toml`, etc.) at the tarball root. We considered one for the description field and rejected it (see Decisions D2); the "namespace IS the manifest" rule in CLAUDE.md stays intact.
3. Reserving plugin subverb names (`install`, `uninstall`, `help`) at the wire layer. We considered this too and rejected it (see Decisions D3); a plugin author who wants to ship `tai foo install` can still do so, but it cannot place assets — the mandatory `assets/` rule makes the host's `SyncAssetsToTargets` the only path that touches target dirs.
4. A migration tool that cleans up the stale `~/.claude/commands/tai-triage`-shaped pollution from triage v0.1.0 installs. The upgrade to the new triage will wipe-and-replace its own namespace; legacy files under `~/.claude/commands/tai/` that came from the old self-installer are left for the operator (or a future `tai plugins doctor`).
5. A new top-level `first-run` capability. The first-run hint is small enough to live under `update-banner`, which already owns the "after the foreground command but before exit" emission slot.
6. Reworking pseudo-version handling for any binary other than the two with a `version` package (`core`, `triage`). Same `resolveVersion` shape is mirrored in both — no shared helper, since the version package's whole point is per-binary isolation (CLAUDE.md "no package-level mutable state" exception).

## Decisions

### D1. Mute background git prompts via env vars, not by removing background poll

**Decision**: pass `GIT_TERMINAL_PROMPT=0`, `GIT_ASKPASS=/bin/echo`, and `GCM_INTERACTIVE=Never` only to background git invocations in `core/internal/sync/poll.go`. Foreground `tai sync` git calls in `clone.go` are unchanged.

**Alternatives considered**:

- *Detach stdin from the background process.* `cmd.Stdin = nil` would stop the read, but git still writes the "Username for..." prompt to stderr before reading — the flash would remain.
- *Skip the poll entirely when no credential helper is configured.* Detecting "no helper configured" is fragile (helpers can be set per-repo, per-host, via includeIf in gitconfig, by `gh auth setup-git`). The env-var approach is robust to all of those.
- *Switch to a Go-native git client (go-git).* Out of scope; would also lose the user's existing credential-helper integration.

The env vars are exactly the documented mechanism for non-interactive git. Credential helpers (osxkeychain, gh, GCM-cached, `.netrc`) still resolve silently because they run BEFORE the interactive fallback. Only the prompt path is blocked. Users without a credential helper (the operator running this change) will see the background poll fail silently — visible only via `tai_data/state/update-check.json`. The foreground `tai sync` still prompts and works.

### D2. `--help-summary` as a wire verb, NOT a static manifest file

**Decision**: add a new wire-contract verb `<plugin> --help-summary` that writes a single-line description to stdout. The host captures it during install/update and persists it as `description` in `plugins.json`. Plugins must implement the verb; install fails with `PLUGIN_HELP_SUMMARY_FAILED` if the call errors or returns >1 KB.

**Alternatives considered**:

- *Static `plugin.yaml` at tarball root.* Cleanest, but breaks the "namespace IS the manifest" rule in CLAUDE.md and forces every future plugin to ship a fourth file (binary + `assets/` + LICENSE + manifest).
- *Lazy: exec `<plugin> --help-summary` on every `tai --help` call.* Cheap per-plugin but linear in installed plugin count; pathological if a plugin crashes (we'd swallow it, but the cost compounds).
- *Description hardcoded in the built-in registry.* Works for first-party plugins only; third-party plugins (which we're explicitly making first-class via the `--source` flag) would have empty descriptions.

The wire-verb approach reuses the subprocess-exec model the host already uses for invocation. The description lives in `plugins.json` (append-only schema, already documented in CLAUDE.md). Lookup is local-only at help time. Adding the verb to `pkg/taiplugin` SDK means a one-liner for plugin authors (`taiplugin.HelpSummary("...")` before `cli.Run`).

### D3. Mandatory `assets/` directory, NOT reserved subverb names

**Decision**: install/update fails with `PLUGIN_ASSET_MISSING` if the tarball has no top-level `assets/` directory. Empty `assets/` is fine; this lets pure-binary plugins opt out cleanly. Combined with a hard CLAUDE.md rule — plugins MUST place all target-bound files through the tarball `assets/` directory; they MUST NOT write directly to target dirs from their own subcommands — this is the trust boundary.

**Alternatives considered**:

- *Reserve `install`, `uninstall`, `help` as plugin subverb names; reject installs that ship them.* Brittle — a malicious or naive plugin can call its self-installer `install-commands`, `bootstrap`, `setup-tai`, anything. The verb-name check would catch the specific shape triage v0.1.0 used, but not the general pattern.
- *Cross-check at install time that `<target.commands>/tai-<plugin>/` contains N files after sync, where N = `len(tarball/assets/commands)`.* Effect-based; misses the cause when N=0. Adds churn to assets that don't ship commands at all.
- *Both reserved-subverb check AND empty-assets check.* Belt-and-suspenders; rejected because the assets check is the load-bearing one and the subverb check makes the surface area larger without catching anything the assets check misses (a plugin shipping `assets/` but ALSO writing directly to target dirs is undetectable at install time anyway).

The mandatory empty-assets requirement is intentionally cheap to satisfy: `mkdir plugins/foo/assets && touch plugins/foo/assets/.gitkeep`. For first-party plugins in this repo we'll add a Make target that verifies all `plugins/<name>/assets/` directories exist as a pre-commit check (one-line guard).

### D4. Pseudo-version detection by regex, not by `golang.org/x/mod/module`

**Decision**: in `resolveVersion`, detect Go pseudo-versions with a regex matching the canonical form — `^v\d+\.\d+\.\d+(-[^.]+)?[.-]\d{14}-[0-9a-f]{12}$` — and return `linked` (`dev`) instead. Real semver releases (`v0.6.0`, `v0.6.0-rc.1`, `v0.6.0+meta.5`) pass through to `info.Main.Version`.

**Alternatives considered**:

- *Import `golang.org/x/mod/module.IsPseudoVersion`.* Tiny module, exact upstream semantics — but adds a transitive dep just for one regex. Today the project's only direct deps are `urfave/cli` + standard library; adding `x/mod` is out of proportion for this fix.
- *Cover the pseudo-version pattern by checking `info.Main.Version == "(devel)" || strings.Contains(v, "-0.")`.* Loose — `-0.` appears in legit prereleases.
- *Pass the linker-ldflags string when `runtime/debug.ReadBuildInfo` is missing but BuildInfo is detected as a non-tagged build.* Wrong abstraction; this is what `linked != "dev"` already covers.

The regex lives in the version package next to `resolveVersion`, table-tested via the existing `TestResolveVersion` table. Pseudo-version format is fixed by the Go toolchain and unlikely to drift.

### D5. Third-party trust = sha256(plugins.yml whole-file), not (third-party subset)

**Decision**: at `tai sync`, if `plugins.yml` exists and contains ≥1 third-party entry, compute `sha256(file bytes)` and compare against `state/trust.json[<repo-url>]`. On mismatch (or absent), prompt once; on yes, store the new hash. Single prompt covers all third-party entries in one go.

**Alternatives considered**:

- *Hash the third-party subset only.* User explicitly rejected this (logged): "we just have to add logic in there for that to only happen for external plugins. ... Keep a hash of the plugins.yaml anytime it changes and there is at least one external plugin." Whole-file hash means internal-plugin additions re-trigger the prompt — acceptable cost for simpler logic.
- *Per-plugin trust entries.* User rejected ("far too much").
- *No persistence — prompt every sync.* Annoying. The hash gives us "ask once per change."

`state/trust.json` shape: `{"trust":[{"repo-url":"https://...","plugins-yml-sha256":"abcdef..."}]}`. Same array-of-objects shape as `plugins.json` so future fields are append-only. The `--trust-third-party` flag bypasses the prompt and updates the cache as if the user said yes — useful for CI/scripted setups.

The actual prompt copy: shown once, lists every third-party entry by `<host>/<org>/<repo>` (NOT by name, since the name comes from the plugin itself which we haven't downloaded yet). User answers `[y/N]` for the whole batch; no per-plugin granularity. Non-TTY without `--trust-third-party` fails `PLUGIN_THIRDPARTY_UNCONFIRMED` without prompting.

### D6. First-run hint lives under `update-banner`, not its own capability

**Decision**: the first-run hint reuses the `update-banner` capability's "after the foreground command but before exit" emission slot. State is a separate marker file (`state/first-run.json`) so it doesn't intermingle with the update-check JSON. The hint and the update-banner are mutually exclusive on first run (we print the first-run hint AND skip the update banner that first time; otherwise the banner takes over).

**Alternatives considered**:

- *New `first-run` capability.* Pure overhead; one rule, one state file, no other surface area.
- *Emit hint from `cmd/tai/main.go` directly, no capability.* Hides the rule from spec readers. Tests would have to drive the entry point.

The marker file shape: `{"first-run":"<ISO-8601 UTC timestamp>"}`. Existence is what matters; the timestamp is informational. Writing the marker is best-effort — failure to write doesn't abort the command, but the hint may print again next run (acceptable failure mode).

### D7. Triage migration: delete the self-installer, ship `assets/` properly, no compatibility shim

**Decision**: deletion-only. Remove `plugins/triage/internal/cmd/install.go`, the whole `plugins/triage/internal/installer/` package, the install/uninstall wiring in `plugins/triage/internal/cmd/root.go`, and the bundled-FS embed pointing at `plugins/triage/assets/commands`. After this change triage v0.2 has the same shape as any other plugin: a binary that handles its own subcommands + an `assets/` directory routed by the host.

**Alternatives considered**:

- *Keep `tai triage install` as a deprecated alias that delegates to `tai plugins update triage`.* Adds permanent dead surface area for a six-week-old plugin with one user.
- *Migration script: on first run of triage v0.2, scan `~/.claude/commands/tai/` for triage-shipped files (import/triage/verify) and delete them.* Risky — those files may have been edited by the user, and the file names are generic enough to collide with legitimate user content.

The cost of "no migration" is small: the operator sees stale `~/.claude/commands/tai/{import,triage,verify}.md` after upgrading. Documented in the changelog; cleanup is a one-liner `rm`. Not worth dedicated tooling.

### D8. Asset filename rewrite (triage `/tai:foo` → `/tai-triage:foo`) is content-only

**Decision**: rewrite the 31 references in `plugins/triage/assets/commands/{import,triage,verify}.md` mechanically. No spec impact — the on-disk routing rule already says `<target.commands>/tai-<plugin>/`, the implementation already follows that, the only thing wrong is what the markdowns claim about themselves.

**Alternatives considered**:

- *Templated rewrite at install time.* Adds an asset-rewrite step to `SyncAssetsToTargets`. Overkill for a one-shot content cleanup; the file-author's plugin name is known at authoring time.

### D9. Post-install hint is a one-liner, not a checked convention

**Decision**: after `tai plugins install <name>` and `tai plugins update <name>` succeed, print exactly: `→ Run \`tai <name> help\` to learn how to use <name>.` Plugin owns the AI-tool-specific orientation in its own help output (which can name slash-commands, skills, etc. as appropriate for the AI tool the plugin targets).

**Alternatives considered** (and rejected by the user):

- *Scan plugin's `assets/commands/` for `learn-<plugin>.md` and reference it.* User: "we shouldn't hard-code the AICode code anywhere ... it's a lot easier than you're making it."
- *Plugin ships `onboarding.txt` and we print its contents.* Same overcomplication.
- *No hint at all.* Loses the discovery surface.

The chosen hint is content-free about the AI tool; the plugin's own help is where the AI-specific guidance lives. tai stays AI-agnostic.

## Risks / Trade-offs

- **[Risk]** Existing plugin authors (today: only triage) need to add `--help-summary` and ship `assets/`. Plugins shipped before this change cannot install on a tai with these checks. → **Mitigation**: triage is the only such plugin and ships from this repo; we land the proposal + triage rebuild together. The error messages name the missing piece exactly so a third-party plugin author who hits the check knows what to fix.
- **[Risk]** A third-party plugin that ALSO ships `assets/` AND writes to target dirs from its own subverbs bypasses the host's namespacing. → **Mitigation**: documented as a rule in CLAUDE.md and explicitly out of scope per Non-Goals (4). Trust prompt is the actual safety boundary; a third-party plugin that doesn't follow the rule is a code-review issue, not a runtime check.
- **[Risk]** The whole-file `plugins.yml` hash re-prompts when only internal plugins change. → **Mitigation**: accepted explicitly by the user. Simplicity > optimal prompt cadence.
- **[Risk]** Pseudo-version detection misses an edge case (e.g. Go toolchain changes the format in a future minor). → **Mitigation**: table-tested. If Go ever changes the pseudo-version format, the test will start passing pseudo-versions through as "real versions" — visible regression that the next operator catches before merging the update.
- **[Risk]** First-run marker write fails (HOME unwritable, permission denied). → **Mitigation**: best-effort; hint may re-print next run. Documented in spec.
- **[Risk]** `GIT_ASKPASS=/bin/echo` is technically a shell-dependent path on Windows. → **Mitigation**: pair it with `GIT_TERMINAL_PROMPT=0` — that alone blocks the prompt on every platform; `GIT_ASKPASS` is belt-and-suspenders. The whole git-poll feature is degraded but safe on Windows even without the askpass override.
- **[Trade-off]** Adding `description` to `plugins.json` makes the schema slightly heavier. → **Accepted**: append-only field, future-proof, removes the need for ad-hoc lookups.
- **[Trade-off]** Trust cache is per-repo-url, not per-source-repo-content. A repo that changes ownership (and thus `repo-url`) re-prompts. → **Accepted**: identifier matches what the operator types.

## Migration Plan

1. Land the proposal + spec deltas + tests for all eight bugs in one PR.
2. Tag core `vX.Y.Z` (likely `v0.2.0` — the breaking change to plugin wire contract).
3. Tag triage `plugins/triage/v0.2.0` from the same commit. The new triage no longer ships `install`/`uninstall` subverbs; the goreleaser config bundles `assets/`.
4. Update `dmastrorillo/homebrew-tap` via `make release-core`.
5. The first time an existing user runs `tai plugins update triage`, the new wire-contract check kicks in; triage v0.2.0 satisfies it. Triage assets land at `<target.commands>/tai-triage/` per spec. Stale files at `<target.commands>/tai/{import,triage,verify}.md` are left behind for the operator to `rm` (changelog includes a one-line cleanup snippet).
6. Operators with existing `state/update-check.json` see no migration impact — that file shape is unchanged. New files `state/first-run.json` and `state/trust.json` are created on first run after the upgrade.

**Rollback**: revert the PR. New state files (`first-run.json`, `trust.json`) are forward-only; they don't break the previous tai binary, which ignores them. The `plugins.json.description` field is similarly ignored by the old binary. No data-loss surface.

## Open Questions

None remaining. Five rounds of grilling closed every decision.
