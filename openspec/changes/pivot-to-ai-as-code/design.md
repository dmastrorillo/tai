## Context

Today's `tai` is a single-purpose Triage AI CLI built around an embedded SQLite store and a bundled-command installer. It has formal infrastructure that we want to preserve and re-use: an OpenSpec → BDD (`test-cases.md`) → TDD pipeline, an XDG-respecting data directory, a stable error-code taxonomy in `internal/errcode`, an error-template writer in `internal/cliout`, an end-to-end command-test harness in `internal/cmdtest`, and an exit-code mapping in `internal/exitcode`. None of this discipline goes away.

What changes is *what tai is for*. After the pivot, the core CLI is an AI-agnostic, unopinionated distribution layer for team-curated AI assets (Claude Code skills, commands, agents). Today's Triage AI behaviour survives as the first **plugin** under the new model. Plugins are deliberately the opinionated layer; core stays unopinionated.

Nothing has shipped to end users yet — this is a free moment to restructure.

## Goals / Non-Goals

**Goals:**

- A unopinionated CLI that distributes AI-as-code assets from a configured git source repo to one or more local target directories, with sync semantics that protect user-authored content and surface team changes.
- A plugin model that accommodates opinionated, code-bearing extensions (commands, agents, databases, CLI surface) without requiring core to know plugin internals.
- A wire-level plugin contract documented well enough that plugin authors in any language (Go, Rust, Python, anything with an executable on disk) can implement it; a Go SDK at `pkg/taiplugin` for the common case.
- Public framework packages at `pkg/` so plugin authors can produce errors that look identical to TAI's own.
- Stability and predictability of CLI output — humans read it, AI agents parse it, both rely on it being consistent across versions.
- Preservation of every TC-ID and OpenSpec proposal in the existing codebase; nothing is lost, only relocated.

**Non-Goals:**

- Supporting AI tools beyond Claude Code as a first-class concern. The design is AI-agnostic because directories are configurable, not because we ship adapters. Other tools work if they happen to consume the same skill/command/agent layout from somewhere on disk.
- Self-update of the `tai` binary. Users update via the package manager they installed it with (`brew upgrade`, `go install ...@latest`, etc.). TAI tells them what to run; it does not run it.
- A central plugin registry. The "registry" is a hard-coded map of first-party names → sources in core. Third-party plugins specify explicit sources.
- Dynamic Go plugins (the standard library's `plugin` package). Cross-platform reliability is poor and version coupling is fragile. We use subprocess execution instead.
- Backwards compatibility with the pre-pivot CLI surface. Nothing has been released; no end-user is affected.

## Decisions

### D1 — AI-agnostic via config, not via a provider abstraction

The CLI does not interpret skill/command/agent semantics. It moves files. Where they go is controlled by the `targets` array in config — each entry has a `root` and optional sub-paths for the three category buckets. Multiple targets are supported (one TAI install can fan out to `~/.claude` and `~/.opencode` simultaneously).

**Rejected:** a provider interface (`type Provider interface { CommandsDir() string; SkillsDir() string; ... }`) with a Claude-Code implementation. Premature abstraction with no second consumer; bakes vocabulary in code where config suffices.

### D2 — Co-required `repo-url` and `targets`; falsy sub-paths skip with a warning

Both top-level config fields are individually optional, jointly required for any operation that writes (`tai sync`, `tai install-commands`, `tai plugins ... install/update`). Omitted sub-paths inside a target default to standard names (`skills`, `commands`, `agents`); to skip a category, set the sub-path to a falsy value (empty string, `false`), which causes a console warning when the corresponding source folder has content.

**Rejected:** treating any omitted sub-path as a skip. That would force every target to enumerate every category, which is noisy and error-prone.

### D3 — M1 sync (existence-only overwrite detection) + cumulative manifest for prune

`tai sync` compares only file existence at the destination, not content bytes. Every existing path goes into a batched single prompt of "the following will be overwritten" and `-y` skips it. A per-target manifest in TAI's data dir records every path TAI has installed and not yet pruned; on each sync the diff of `manifest_existing - source_current` yields orphans, which `tai sync --prune` deletes. Without `--prune`, orphans persist across syncs and the sync summary surfaces a "N orphans pending" line.

**Rejected:**

- **M2 (content-diff, stateless):** would have collapsed second-sync-no-changes into a silent no-op, but loses the ability to track orphans across multiple syncs without a manifest. Once we have a manifest for prune anyway, M2's diff savings don't justify the byte-comparison code path.
- **M3 (full manifest with content hashes):** distinguishes "user edited a previously-synced file" from "user authored their own file at this path." Both result in the same prompt with M1, so the discrimination earns nothing for the prompt UX. Defer until a real use case appears.

The flag chosen is `-y` (matches CLI conventions); `--accept-overwrites` is not used.

### D4 — Eager fetch + cache fallback + background poll, no `--offline`

`tai sync` always attempts `git fetch` on the clone before reading it. On network failure, it logs a one-line warning to stderr naming the failure and the cache's last-successful-fetch timestamp, then proceeds against the cache. The background update-check daemon (see D11) polls the same upstream for change-detection notifications between syncs.

**Rejected:** a `--offline` flag. Network failure handled implicitly produces the same effect; an explicit flag adds surface without buying anything.

### D5 — Plugins are subprocess executables under a hard-coded path, identified by directory name

A plugin is `~/.local/share/tai/plugins/<name>/` containing a binary (`<name>` or platform equivalent) and an `assets/` directory. The directory name is the single source of identity — it is also the top-level CLI verb (`tai <name> <args...>`), the namespace prefix for the plugin's skill/agent assets (`tai-<name>-*`), and the subdirectory name for its commands (`commands/tai-<name>/`). No `plugin.yml` manifest in the plugin itself. TAI invokes plugins via subprocess; stdin/stdout/stderr/exit pass through.

**Rejected:**

- **Plugins as Go packages compiled into `tai`:** would force first-party plugins to ship with core, and third-party plugins to fork-and-rebuild. Breaks the install/update story.
- **Go `plugin.Open()` shared libraries:** broken on macOS/Windows in practice; no real-world Go projects rely on it.
- **gRPC `hashicorp/go-plugin`:** heavyweight; out of proportion for what TAI needs.
- **A plugin manifest file alongside the binary:** the directory name already carries every piece of metadata we need. A manifest would duplicate state and create a new validation surface.

### D6 — Plugin asset scoping eliminates collision prompts and manifests

Because plugins must name their skills/agents `tai-<plugin>-*` and TAI routes their commands into `commands/tai-<plugin>/`, plugin assets cannot collide with user-authored content (which by convention does not use the `tai-*` prefix). TAI overwrites freely within the namespace on every install/update — the namespace itself is the manifest. To "delete" a plugin's assets, TAI lists every file matching the namespace and removes them, then writes the new ones.

Installation-time validation: skill and agent filenames in the plugin's `assets/skills` and `assets/agents` MUST start with `tai-<plugin>-`. Install fails with a `PLUGIN_ASSET_NAMING` error if any don't. Command filenames are not validated — TAI's routing enforces the `commands/tai-<plugin>/` subdirectory irrespective of authored name.

**Rejected:** a per-plugin manifest mirroring the source-repo manifest. Adds state without benefit; namespacing makes the manifest unnecessary.

### D7 — R2 registry: built-in map for first-party plugins, explicit source for third-party

Core ships a hard-coded `map[string]Source` of first-party plugin names. `triage` is the only entry today, resolving to this repo's GitHub Releases. For third-party plugins, both `plugins.yml` and the install CLI accept an explicit source spec (`<host>/<org>/<repo>[/<subpath>]@<version>`). Distribution is via GitHub Releases of the source repo; release assets follow `tai-plugin-<name>-<os>-<arch>` (with `.exe` on Windows). No compile-from-source.

**Rejected:**

- **No registry (R1) — explicit source always:** verbose for the 95% case (a user wants Triage AI; nobody wants to type a URL for that).
- **Convention-based discovery (R3) — `github.com/<org>/tai-plugin-<name>`:** brittle when orgs rename or conventions slip; legitimises name-squatting.

Adding a new first-party plugin is a documented two-step workflow (register entry + cut release), recorded in CLAUDE.md.

### D8 — Single Go module; public framework under `pkg/`

One `go.mod` at the repo root. Multiple binaries built from `core/cmd/tai/` and `plugins/<name>/cmd/<name>/`. Shared framework packages — `errcode`, `cliout`, `exitcode`, and a new SDK `taiplugin` — live under `pkg/` so any external Go module can import them. Anything under `pkg/` is on a stability contract (append-only error codes; no renaming or repurposing exported symbols).

**Rejected:** multi-module with `go.work`. Useful when plugins need independent dep versions; YAGNI now, with the migration trigger being "a first-party plugin needs a conflicting dep version." Third-party plugins live in third-party repos anyway, so they don't bear on this decision.

#### D8a — YAML library: `gopkg.in/yaml.v3`

Phase 1's config-loader pulls in a YAML library. We picked `gopkg.in/yaml.v3` over the other candidates:

- **`gopkg.in/yaml.v3`** *(chosen)*: ubiquitous in the Go ecosystem, stable API since 2020, the `Node`-based decoder gives us round-trip control if we ever need it, supports comments via `Node.HeadComment`. Maintenance has slowed since 2022 but no critical bugs are outstanding and the package is feature-complete for tai's needs (key-value loading, `omitempty`, pointer-vs-nil semantics for sub-paths).
- **`go-yaml/yaml/v4`** *(rejected for v1)*: API still in flux as of writing; the v3→v4 migration is non-trivial and we'd rather have a stable foundation. Revisit when v4 reaches a release marked stable.
- **`sigs.k8s.io/yaml`** *(rejected)*: defers to `yaml.v2` under the hood and converts via JSON tags. Adds a JSON-tag burden on our structs that we don't need, and v2 lacks the round-trip capabilities we'd want for `tai config edit`'s byte-fidelity goal.
- **`cuelang.org/go`** *(rejected)*: vastly more powerful than we need; pulls a multi-MB dependency tree.

Migration trigger: when yaml.v3 stops receiving security fixes OR v4 reaches a release marked stable AND the migration cost (one config-loader rewrite) is justified by a concrete need. Until then, this choice is intentional and stable.

### D9 — Output convention: stdout = data, stderr = conversation, single mode, string error codes

No `--json` / machine mode. Stdout carries data — prose headers, tabular rows, results. Stderr carries everything else — error templates, progress, prompts, the update banner. Errors flow through `cliout.WriteError` with the existing template:

```
Error: <one-line summary>

What to do:
  • <remediation step>
  …

[exit <N>: <ERROR_CODE>]
```

Codes are UPPERCASE_SNAKE_CASE, append-only, never renamed. The footer regex `^\[exit \d+: [A-Z][A-Z0-9_]*\]$` is the AI's anchor and is verified by test invariants. New codes for the new capabilities are appended to `internal/errcode`; nothing existing is renamed.

**Rejected:**

- **S2 (AI-first terse default):** hostile to humans at the CLI.
- **S3 (`--json` mode):** double surface to author and maintain; LLMs parse prose well, and discipline gives us most of the parseability benefit.
- **Numeric error codes:** self-documenting strings win in logs.

### D10 — Workflows are markdown plans the AI consumes, not executable scripts

`tai workflow run <name>` reads `<clone>/workflows/<name>.yml` and emits a markdown plan to stdout: a required-tools enumeration (with `kind: skill` or `kind: command`), an ordered step list, and an explicit "if any tool is unavailable, abort and report" failure mode. The AI is responsible for verifying availability and aborting — TAI describes the contract, it does not enforce it (it can't; the tool roster is owned by the AI session).

Workflow file format is YAML. Step `kind` is `skill | command` — `agent` is rejected because agents are not directly invokable; they are reached transitively from skills/commands. Output renders both kinds as `/<name>` since skills and commands are slash-invokable in this user's setup.

`tai workflow list` enumerates all workflows; the name "list" is reserved (a workflow may not be named `list`).

**Rejected:**

- **A built-in `/tai-workflow` slash command auto-installed at `tai sync` time:** ergonomic but mixes TAI's distribution layer with TAI's own integration assets. Instead, `tai install-commands` is an explicit user gesture; the slash command lives in the bundled `assets/` shipped by core and only enters a target when the user opts in.
- **Workflows in JSON:** YAML accepted because workflows are human-authored and benefit from comments.

### D11 — Update banner: background check every 6h, once-per-day stderr aggregation

A fire-and-forget background goroutine on each invocation refreshes a cache at `~/.local/share/tai/state/update-check.json` if older than the configured interval (default 6h). On each invocation, if the cache lists pending updates AND no banner has been printed today (gated by a `last-banner-date` field), TAI prints an aggregated stderr banner naming any of: a newer `tai`, newer installed plugins, or newer source-repo `main`. No acknowledgement required — the banner naturally re-fires the next calendar day.

TAI itself is not self-updating. The banner names the package-manager command the user should run.

**Rejected:**

- **Synchronous check on every invocation:** slow start.
- **Banner on every invocation:** notification fatigue.
- **Dedicated `tai status` command only, no banners:** updates the user can't act on get missed.
- **Banner with `tai update ack`:** added surface for negligible benefit; the daily roll-over already breaks the prompt loop.

### D12 — Auth: defer to git for source repo; opportunistic `GITHUB_TOKEN` for plugin downloads

TAI never manages source-repo credentials. `git fetch` does whatever git's credential machinery (SSH agent, credential helper, Keychain) tells it. Errors bubble through `cliout.WriteError` with a `REPO_FETCH_FAILED` code whose "what to do" bullets point at git config.

Plugin downloads from GitHub Releases: if `GITHUB_TOKEN` is set, TAI sends it as a Bearer token. If not set, anonymous. On 401/403 the error message tells the user to set `GITHUB_TOKEN`. The token is never required up front. Other hosts (GitLab, etc.) get analogous env vars when a real plugin demands them.

**Rejected:** TAI reading from `gh auth status` or any other CLI tool. Keeps the dependency surface explicit.

### D13 — Config UX: dedicated subcommands + editor fallback, no path-syntax setter

`tai config show` prints YAML. `tai config edit` opens `$EDITOR`, creating a commented template on first call. `tai config set <key> <value>` accepts scalar top-level keys only (`repo-url`, `update-check-interval`); nested or array paths return `CONFIG_KEY_NOT_SCRIPTABLE`. `tai config target {add, list, remove}` handles the targets array. The config file lives at `$XDG_CONFIG_HOME/tai/config.yml` (overridable by `TAI_CONFIG`), created lazily on first write — never on read or on first `tai --help`.

**Rejected:** a path-syntax setter (`tai config set targets[0].root ...`). Error-prone; users get the bracket syntax wrong; permissive parsing creates inconsistency, strict parsing creates friction. Dedicated subcommands are self-documenting in `--help`.

### D14 — `tai repo init` scaffold, always git-inited, no auto-wiring of local config

`tai repo init <path>` creates the full source-repo template at `<path>`, including per-folder READMEs that explain naming conventions and worked examples, then runs `git init` and makes the initial commit `Initial TAI source-repo scaffold`. No `--no-git` flag; the directory is intrinsically a git repo. The author-machine's local TAI config is not modified — the operator is expected to push to a remote and run `tai config set repo-url ...` after, since the author-machine is rarely the canonical consumer.

A successful scaffold prints a next-steps block on stdout with the exact commands the operator runs next.

## Risks / Trade-offs

- **Risk:** Large blast-radius pivot in one OpenSpec change. Bigger diff, more places to get wrong, harder to bisect a regression.
  **Mitigation:** the migration plan (below) lands the work in ordered phases under a single proposal. The first phase is a pure no-op refactor (move files, no behavior change) — all existing tests still pass before any new capability lands. Each new capability has its own task block in `tasks.md`, its own BDD cases in the right `test-cases.md`, and its own commit if possible. The pipeline discipline is preserved.

- **Risk:** Splitting `test-cases.md` per component loses cross-component visibility. Today the single file makes it obvious where each TC-ID lives; after the split, a TC-ID-only reference (`TC-CMD-015`) is ambiguous between core and triage.
  **Mitigation:** TC-IDs continue to use category prefixes (`CMD`, `CFG`, ...) that are already disambiguated by category, not by component. Where ambiguity could arise for new IDs, we add a component-aware prefix (`TC-CORE-SYNC-001`, `TC-TRIAGE-IMP-001`). Existing IDs keep their current shape; no renumbering.

- **Risk:** M1 prompts on every overwrite, even no-op syncs. After the first sync of 50 skills, the second sync still lists all 50 as "would overwrite" until the user explicitly accepts or uses `-y`. Friction over time.
  **Mitigation:** documented `-y` and the `--accept-overwrites`-style escape hatch is `-y` itself, the standard convention. If the friction proves real, a follow-up proposal can layer M2 on top — the manifest already exists, so the additional state is small. Explicit non-goal for now.

- **Risk:** Public `pkg/` API commits us to stability. Future renames or breaking changes require a major version bump.
  **Mitigation:** start `pkg/` small. Move only what is genuinely needed by plugin authors today (`errcode`, `cliout`, `exitcode`, `taiplugin`). Anything tentative stays internal.

- **Risk:** Plugin naming namespace (`tai-<plugin>-*`) requires plugin authors to be disciplined. A plugin author who ships a skill named `cool-skill` (without the prefix) will fail at install — a poor first impression.
  **Mitigation:** install-time validation surfaces the failure with a precise error code and a "what to do" pointer to the naming rule. CLAUDE.md additions and the README's plugin-authoring section call out the rule prominently. The `pkg/taiplugin` SDK can include a helper that asserts namespace correctness at plugin-build-time so the failure surfaces during plugin development, not at user install.

- **Risk:** Subprocess invocation has latency overhead per call. For Triage AI commands that today are in-process, the new world adds an `exec` and a process bootstrap to every command.
  **Mitigation:** typical Go binary startup is sub-50ms; for an interactive CLI this is invisible. We're not in a hot loop. If a future plugin needs to be called repeatedly inside another command, the env-var contract leaves room for a future "plugin daemon" mode without affecting the simple case.

- **Risk:** Update banner causes stderr noise during AI consumption — an AI agent piping `tai standards list` may be confused by an unexpected `[tai] Updates available: ...` line.
  **Mitigation:** the banner is on stderr (not stdout), is one-line, machine-parseable (prefixed `[tai]`), and limited to one fire per day. AI agents parsing stdout see exactly the data they asked for. AI agents that ingest stderr see one extra parseable line per day — within tolerance.

- **Risk:** Source-repo authors may name workflows or standards "list", "load", or "run". Reserved-name collision.
  **Mitigation:** validators reject reserved names with clear errors. The same validators reject case-insensitive duplicate names (`devOps:security` vs `devops:security`) since lookups lowercase.

## Migration Plan

The pivot is a single OpenSpec change but lands as ordered phases. Each phase keeps `go test ./... && go vet ./... && gofmt -l .` green at every commit per `CLAUDE.md`.

1. **Phase 0 — repo restructure (no behavior change).** Move `cmd/tai-ledger/`, `cmd/tai/`, and all of `internal/` per the new layout. Framework packages (`errcode`, `cliout`, `exitcode`) move to `pkg/`. Triage code moves to `plugins/triage/`. Imports rewritten. Existing tests pass. `test-cases.md` splits into `core/test-cases.md` and `plugins/triage/test-cases.md` with TC-IDs intact. No new behavior.

2. **Phase 1 — core foundation.** New `tai` binary scaffold under `core/cmd/tai/main.go` wiring the new top-level verbs as stubs that return `UNKNOWN_SUBCOMMAND` until each is implemented. New config schema and the `tai config` surface. New error codes appended to `pkg/errcode`. Per-component test files start receiving new TC-IDs.

3. **Phase 2 — repo lifecycle.** `tai sync` (clone, fetch with cache fallback, M1 overwrite, manifest, `--prune`, `-y`). `tai repo init`. Drops the new clone into the data dir. End-to-end tests via `cmdtest` harness.

4. **Phase 3 — content surfaces.** `tai workflow list/run`, `tai standards list/load`, `tai install-commands`. All read from the clone; none write outside configured targets.

5. **Phase 4 — plugin host.** `tai plugins list/<name> install/update/remove`. Registry, GitHub Releases fetch, install-time namespace validation, env-var contract, `plugins.yml` auto-install on sync. `pkg/taiplugin` SDK published.

6. **Phase 5 — update banner.** Background fetch goroutine, cache file, once-per-day stderr aggregation. Surfaces TAI, plugins, and source-repo updates.

7. **Phase 6 — Triage plugin migration.** Wire `plugins/triage/cmd/triage/main.go` as a real plugin. Re-point all Triage AI tests to invoke `tai triage <verb>` (existing TC-IDs preserved). Bundled commands repackaged as plugin assets with `tai-triage-` prefix.

8. **Phase 7 — docs and release pipeline.** README rewrite (origin story, problem statement, plugin authoring guide). CLAUDE.md updates including the "add a first-party plugin" workflow. CI workflow extended to build and attach `tai` + each plugin binary across the OS/arch matrix.

Rollback strategy: since nothing has shipped to users, rollback is a `git revert` away. No deployed state to migrate back. The pre-pivot tag is preserved on `main`'s history; reverting drops back to it cleanly.

## Open Questions

- **Plugin SDK API surface for v1.** `pkg/taiplugin` will at minimum parse the env-var contract (`TAI_CLONE_DIR`, `TAI_TARGETS`, `TAI_DATA_DIR`) into a typed `Context`. Should it also re-export `errcode` / `cliout` for convenience, or require plugins to import those separately? Recommend: re-export for ergonomics; revisit if cyclic-dep concerns appear.
- **Reserved-name list precise scope.** Today's list: `config`, `sync`, `repo`, `install-commands`, `workflow`, `standards`, `plugins`, `help`, `version`. Should `init`, `update`, `list`, `load`, `run`, `add`, `remove`, `set`, `show`, `edit` also be reserved (since they're sub-verbs that appear in many contexts)? Recommend: reserve only top-level verbs; sub-verbs are scoped under their noun.
- **GitHub Releases asset naming convention enforcement.** `tai-plugin-<name>-<os>-<arch>` is the format we emit and consume. For third-party plugins, do we require the same convention, or accept any asset name and let the registry entry specify? Recommend: convention is mandatory for v1; revisit if a real third-party plugin can't comply.
- **Behavior of `tai plugins <name> remove` toward plugin runtime state.** If a plugin has its own SQLite database at `<TAI_DATA_DIR>/plugins/<name>/state/`, does `remove` delete that data? Recommend: keep the state (data is precious; plugin reinstall recovers cleanly), warn the user that state remains and name the path to delete manually.
