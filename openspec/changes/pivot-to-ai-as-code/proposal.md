## Why

TAI started as a single-purpose Triage AI tool, but the underlying problem we keep hitting is broader: teams have no clean way to ship a baseline of Claude-Code (and similar AI-tooling) assets — skills, commands, agents — to every developer machine consistently. There is also no neutral substrate for layering opinionated, code-bearing extensions (like Triage AI itself) on top of that baseline.

This change repurposes the existing (unreleased) `tai` codename into an **AI-as-code CLI**: an unopinionated, AI-agnostic core that distributes a git-versioned source repo's assets to configured local targets, plus an opinionated **plugin** layer for code-bearing extensions. The current Triage AI feature set becomes the first plugin under the new model.

Because nothing has shipped to end users, this is the cheapest moment to make the change.

## What Changes

- **BREAKING — Entire CLI surface restructured.** Top-level verbs that today belong to Triage AI (`tai import`, `tai accept`, `tai list`, `tai install`, `tai forget`, `tai mutate`, `tai verify`) are removed from core. They reappear under `tai triage <verb>` once the Triage plugin is wired in.
- **New `tai` core CLI.** Unopinionated AI-as-code distribution layer. Reads a configured **source repo** over git, copies `skills/`, `commands/`, `agents/` into one or more configured **targets** (e.g. `~/.claude/`), exposes `workflows/` and `standards/` lazily via `tai workflow run / list` and `tai standards load / list`. Surface details captured in `design.md`.
- **New config schema.** YAML at `$XDG_CONFIG_HOME/tai/config.yml` with `repo-url`, `targets` (array of `{root, skills?, commands?, agents?}`), and `update-check-interval`. Lazy-created. Edited via `tai config show / edit / set`, with `tai config target add / list / remove` for the array.
- **New sync model.** `tai sync` performs an eager `git fetch` (falls back to last-known-good cache on network failure), then copies assets with **M1 overwrite detection** (existence-only check, no byte comparison; batched single prompt; `-y` skips). A lightweight per-target manifest tracks what TAI installed so `tai sync --prune` can remove assets the source repo no longer ships, without affecting user-authored content.
- **New plugin model.** Plugins are standalone executables under `~/.local/share/tai/plugins/<name>/` plus a sibling `assets/` directory. Their assets use a mandatory namespace (`tai-<name>-*` for skills/agents; `commands/tai-<name>/` subdirectory for commands), so they overwrite freely within that scope without prompts. `tai plugins <name> install/update/remove` and `tai plugins list`. First-party plugins resolve via a hard-coded registry; third-party plugins specify an explicit source. Distribution via GitHub Releases; opportunistic `GITHUB_TOKEN` for private sources.
- **New `tai repo init <path>` scaffold** that creates a templated source repo (skills/, commands/, agents/, workflows/, standards/, plugins.yml, READMEs at every level), automatically `git init`-ed with an initial commit.
- **New `tai install-commands`** that installs TAI's own built-in slash commands (currently `/tai-workflow`-style helpers) into a `tai/` subdirectory of every configured target's commands path.
- **New update-notification banner.** Background fire-and-forget check every 6 hours (configurable); a once-per-day stderr banner aggregates available updates for TAI itself, installed plugins, and the source repo. TAI does not self-update — it tells the user the package-manager command for that.
- **New monorepo layout.** Top-level `core/` holds the `tai` binary; `plugins/<name>/` holds first-party plugins (currently just `triage/`); `pkg/` exposes the shared framework (`errcode`, `cliout`, `exitcode`, `taiplugin`) so third-party plugins can `import` them. Single Go module at root. Existing `internal/...` paths move accordingly.
- **New output convention codified.** Stdout = data; stderr = conversation. Errors via the existing `cliout.WriteError` template with string error codes (append-only, never renamed) and an invariant `[exit N: ERROR_CODE]` footer regex. This is already in code; the change formalises it for the new commands and the plugin contract.
- **Wire-level plugin contract.** TAI passes `TAI_CLONE_DIR`, `TAI_TARGETS` (JSON), `TAI_DATA_DIR` to plugin subprocesses; pass-through of stdin/stdout/stderr/exit; same error-template + footer rules expected back. Plugin authors in any language can implement this; Go authors can import `pkg/taiplugin`.
- **`plugins.yml`** added at the source-repo root: an additive list of plugins TAI should auto-install on `tai sync`. Removing an entry does not uninstall on developer machines (additive, not authoritative).
- **`test-cases.md` splits per component.** Existing root-level file is replaced by `core/test-cases.md` (carrying forward core/foundation cases) and `plugins/triage/test-cases.md` (carrying forward Triage AI cases). TC-IDs are unchanged; tests move along with their components.

## Capabilities

### New Capabilities

- `config`: TAI's local configuration — file location, lazy creation, schema (`repo-url`, `targets`, `update-check-interval`), and the `tai config` CLI surface.
- `repo-sync`: cloning the source repo, eager fetch with cache fallback, M1 existence-based overwrite detection, manifest tracking, `tai sync --prune` deletion handling, the batched overwrite prompt and `-y`.
- `repo-init`: `tai repo init <path>` scaffolding — directory structure, per-folder READMEs, automatic `git init` + initial commit, the next-steps print block.
- `workflows`: workflow file format (YAML), the colon-namespaced naming scheme, `tai workflow list`, and the markdown plan emitted by `tai workflow run <name>` that an AI consumes — including required-tools enumeration and fail-loud delegation.
- `standards`: standards file format (markdown + optional frontmatter `description`), colon-namespaced lowercased addressing, lookup by name across nested directories, `tai standards list` and `tai standards load <name>`.
- `install-commands`: `tai install-commands` behaviour — discovering configured targets, writing TAI's bundled slash commands into `<target>/<commands>/tai/`, validation, and re-run idempotency.
- `plugin-host`: physical plugin layout, the directory-name-as-identity rule, the `tai-<name>-*` namespace contract, install-time validation, the registry (R2 hybrid: built-in for first-party + explicit-source for third-party), GitHub Releases fetch, `GITHUB_TOKEN` handling, `tai plugins list / <name> install / update / remove`, the `plugins.yml` additive auto-install at sync time, and the wire-level env-var contract passed to plugin subprocesses.
- `update-banner`: the background update-check cadence, the cache file format, and the once-per-day aggregated stderr banner spanning TAI itself, installed plugins, and the source repo.

### Modified Capabilities

- `cli-framework`: extended to host the new commands and to formalise the stdout-vs-stderr discipline for all commands (not just error paths). The existing append-only error-code taxonomy and footer regex are preserved; new error codes for the new capabilities are appended.

## Impact

- **Repo restructure**: every Go source file currently under `cmd/` and `internal/` moves. Core code lands under `core/`; framework packages (`errcode`, `cliout`, `exitcode`, plus a new `taiplugin`) move into `pkg/`; all Triage AI code (`internal/triage`, `internal/import`, `internal/storage`, the existing `internal/cmd/*` triage commands, the bundled-content infrastructure in `internal/cmdframework` and `internal/installer`) moves under `plugins/triage/`.
- **Existing `internal/` import paths break** for everything that moves. The single Go module covers the move with one `go.mod` rewrite; downstream callers (there are none yet — unreleased) need no migration.
- **`test-cases.md` split**: the existing root file is divided. Core/foundation TC-IDs go to `core/test-cases.md`; all Triage AI TC-IDs go to `plugins/triage/test-cases.md`. The CLAUDE.md pipeline doc is updated to point at both.
- **CLI behavior breaks** for everyone who has been testing the current `tai` binary locally — none of the triage verbs work at the top level after this change. Users running today's tai locally would need to invoke `tai triage <verb>` after the migration. Since the tool has not been released, no user is in this position outside of the maintainer.
- **`openspec/specs/` reorganization**: triage-specific specs (`storage`, `import`, `triage`, `triage-command`, `verify-command`, `command-framework`, and the current `install`) become plugin-internal capabilities owned by the Triage plugin. They are not removed but their narrative shifts to "this is how the Triage plugin behaves," not "this is how `tai` behaves."
- **New external public API surface** under `pkg/` (`errcode`, `cliout`, `exitcode`, `taiplugin`). Anything published there is on a stability contract: append-only error codes, no renaming or repurposing of exported symbols without a major version bump.
- **GitHub release pipeline changes**: a release of the tai repo must now produce multiple binaries (core `tai` + each first-party plugin) across the OS/arch matrix. CI workflow needs to build, name, and attach all of them. Naming convention for plugin release assets: `tai-plugin-<name>-<os>-<arch>` (with `.exe` suffix on Windows).
- **Documentation**: README rewritten — name origin story (Triage AI → TAI), problem statement, install, the source-repo pattern, plugin model, plugin authoring. `CLAUDE.md` and `CONTEXT.md` both updated to reflect the new architecture; `CLAUDE.md` adds the "to add a first-party plugin, register an entry + cut a release" workflow.
