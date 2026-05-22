## 1. Phase 0 — Repo restructure (no behavior change)

- [x] 1.1 Move `cmd/tai/` to `core/cmd/tai/` and update its imports
- [x] 1.2 Move `cmd/tai-ledger/` to `plugins/triage/cmd/ledger/` (or retire if no longer needed) and update its imports
- [x] 1.3 Move framework packages out of `internal/` to public `pkg/`: `internal/errcode` → `pkg/errcode`, `internal/cliout` → `pkg/cliout`, `internal/exitcode` → `pkg/exitcode`
- [x] 1.4 Move core-internal helpers to `core/internal/`: `internal/cliexec`, `internal/datadir`, `internal/repoctx` (the parts still useful in core; Triage-specific repoctx behavior moves with Triage)
  - **As-built deviation** (agreed with the user during Phase 0 implementation): Go's `internal/` rule blocks cross-tree imports, so three packages from this task landed at different paths than the literal text:
    - `cliexec` → `pkg/cliexec/` (public). Both `core/cmd/tai/` and `plugins/triage/internal/cmdtest/` use it; placing it under either `internal/` tree would block the other.
    - `datadir` → `plugins/triage/internal/datadir/`. All current callers are triage-side (`plugins/triage/internal/storage` only). Will need promotion to `pkg/` when Phase 1's `core/internal/config` lands and needs data-directory resolution — flagged in a `datadir.go` doc comment.
    - `repoctx` → `plugins/triage/internal/repoctx/`. All current callers are triage-side; the package was always wholly triage-coupled (it parses git-origin URLs for the `--repo` flag flow). No Phase 1 core callers expected.
- [x] 1.5 Move Triage-specific code to `plugins/triage/internal/`: `internal/triage`, `internal/import`, `internal/storage`, `internal/installer`, `internal/cmdframework`, `internal/cmd/*` (all triage-related), `internal/cmdtest`
- [x] 1.6 Rewrite all Go imports across the moved files to their new paths
- [x] 1.7 Split `test-cases.md` into `core/test-cases.md` (foundation/CLI cases) and `plugins/triage/test-cases.md` (triage cases); preserve every TC-ID verbatim. Apply this mapping for the existing TC-CMD category: `TC-CMD-001`, `TC-CMD-002`, `TC-CMD-008` → `core/test-cases.md` (foundation CLI behaviours); `TC-CMD-003` through `TC-CMD-007` → `plugins/triage/test-cases.md` (cmdframework / bundled-command infrastructure, plugin-internal after the pivot). Remove the root-level `test-cases.md` once both per-component files contain every TC-ID and the test suite passes
- [x] 1.8 Update `CLAUDE.md` at this phase (not Phase 7) to reflect the new layout — this is MANDATORY because subsequent phases reading CLAUDE.md would otherwise be misled. Specific edits required: rewrite the "Project Structure" section to reference `core/`, `pkg/`, `plugins/<name>/`; remove the line stating "Production code lives under `internal/`" (or rephrase to allow `pkg/`); update the line "`cmd/tai/main.go` is the only place that translates an error into an exit code" to point at `core/cmd/tai/main.go`; replace every reference to the singular root `test-cases.md` with the pair `core/test-cases.md` / `plugins/<name>/test-cases.md`; the OpenSpec proposal-archiving rules remain unchanged
- [x] 1.9 Update `go.mod` module path if needed; ensure `go build ./...`, `go test ./...`, `go vet ./...`, `gofmt -l .` all return green
- [x] 1.10 Snapshot-test: `go test ./...` passes with exactly the same set of green tests as before the move

## 2. Phase 1 — Core foundation: config and error taxonomy

- [ ] 2.1 Introduce a new `TC-CONF-*` category in `core/test-cases.md` for config-file management (file location, schema, CLI surface — the work of this phase); update the ToC entry. The existing `TC-CFG-*` category remains scoped to data-directory and config-resolution cases (its current meaning); the two are kept distinct to avoid conflating data-dir resolution with YAML config-file management. Add BDD cases for the `config` capability under `TC-CONF-001` onward
- [ ] 2.2 Append new error codes to `pkg/errcode`: `CONFIG_UNWRITABLE`, `CONFIG_INVALID`, `CONFIG_INVALID_REPO_URL`, `CONFIG_KEY_NOT_SCRIPTABLE`, `CONFIG_DUPLICATE_TARGET`, `CONFIG_TARGET_NOT_FOUND`, `TAI_NOT_CONFIGURED` — with exit-code bindings per the spec
- [ ] 2.3 Add a config-loader package in `core/internal/config` that resolves the file path (TAI_CONFIG → XDG_CONFIG_HOME → platform default), parses YAML, validates the schema (repo-url shape, targets array, sub-path defaults, falsy-skip behavior)
- [ ] 2.4 Write red unit + e2e tests for each TC-CFG-* case (path resolution, lazy creation, schema validation, falsy sub-path warning)
- [ ] 2.5 Implement `tai config show` — prints YAML; informational message on missing file; exits 0 either way
- [ ] 2.6 Implement `tai config edit` — creates commented template on first call, opens `$EDITOR`
- [ ] 2.7 Implement `tai config set <key> <value>` — scalar top-level keys only; rejects nested keys with `CONFIG_KEY_NOT_SCRIPTABLE`
- [ ] 2.8 Implement `tai config target add <root> [--skills X] [--commands Y] [--agents Z]` — duplicate-root detection, default sub-paths preserved as absent in YAML
- [ ] 2.9 Implement `tai config target list` — table on stdout; `(no targets configured)` when empty
- [ ] 2.10 Implement `tai config target remove <root>` — exact-match removal; `CONFIG_TARGET_NOT_FOUND` otherwise
- [ ] 2.11 Wire `tai --help`, `tai --version` to work with no config present (already works in foundation; reverify)
- [ ] 2.12 Refactor existing error-template assertions to live in `pkg/cliout` tests; ensure footer-regex invariant test still passes

## 3. Phase 1 — Core foundation: stdout/stderr discipline and reserved verbs

- [ ] 3.1 Add BDD cases for the stdout/stderr discipline in `core/test-cases.md` (TC-CLI-* category)
- [ ] 3.2 Implement a TTY-detection helper in `pkg/cliout` (or `core/internal`) and ensure no ANSI/CR animations on non-TTY stdout
- [ ] 3.3 Add a reserved-verbs registry in `core/internal` that exports the canonical top-level verb list maintained in `specs/plugin-host/spec.md` Requirement: Plugin subprocess invocation; expose it for the plugin host. The list MUST NOT be duplicated inline in this task or elsewhere — fetch it from the spec at implementation time
- [ ] 3.4 Wire the root command to return `UNKNOWN_SUBCOMMAND` for any verb not in the registry and not resolvable as a plugin (the plugin host hook lands later)

## 4. Phase 2 — Repo lifecycle: sync and clone

- [ ] 4.1 Add BDD cases to `core/test-cases.md` for the `repo-sync` capability (TC-SYNC-* category)
- [ ] 4.1a Add BDD cases to `core/test-cases.md` for the background source-repo update-check goroutine (TC-SYNC-* category continues): cache file refreshed when stale, cache file unchanged when fresh, cache file unchanged on poll error. These are the file-observable anchors for the goroutine introduced in task 4.10
- [ ] 4.2 Append `REPO_FETCH_FAILED` to `pkg/errcode` with its exit code
- [ ] 4.3 Implement a clone manager in `core/internal/sync` that creates `<TAI_DATA_DIR>/source/` on first sync via `git clone`, reuses on subsequent syncs
- [ ] 4.4 Implement eager `git fetch` with cache-fallback warning to stderr (one-liner naming last-successful-fetch timestamp)
- [ ] 4.5 Implement M1 overwrite detection: walk the source tree, check destination existence per file, batch into `would-create` / `would-overwrite` / `up-to-date` lists, group by category
- [ ] 4.6 Implement the per-target manifest at `<TAI_DATA_DIR>/manifests/<sha256-of-root>.json`: cumulative; append on write, remove only on prune
- [ ] 4.7 Implement the batched single overwrite prompt on stderr; read y/N from stdin; -y bypasses prompt
- [ ] 4.8 Implement the orphan-count summary on every `tai sync` (whether or not `--prune` was passed)
- [ ] 4.9 Implement `tai sync --prune`: compute `manifest - source` orphans, surface them in the prompt, delete on confirm
- [ ] 4.10 Add a background goroutine for the source-repo update check; result feeds the update-banner state file (consumed in phase 5)
- [ ] 4.11 Write e2e tests using a local git remote fixture (a bare git repo in a temp dir) covering every TC-SYNC-* case

## 5. Phase 2 — Repo lifecycle: `tai repo init`

- [ ] 5.1 Add BDD cases for `repo-init` to `core/test-cases.md` (TC-INIT-* or new category)
- [ ] 5.2 Append `REPO_INIT_TARGET_NOT_EMPTY` and `REPO_INIT_GIT_UNAVAILABLE` to `pkg/errcode`
- [ ] 5.3 Embed the scaffold templates (top-level README.md, per-folder READMEs, .gitignore, plugins.yml commented stub) as Go embedded files in `core/internal/repoinit`
- [ ] 5.4 Implement `tai repo init <path>`: directory checks, scaffold write, `git init`, initial commit, next-steps print block
- [ ] 5.5 Test that local config is not modified after init (TC-INIT-* covers auto-wiring negative case)

## 6. Phase 3 — Content surfaces: workflows and standards

- [ ] 6.1 Add BDD cases for `workflows` and `standards` to `core/test-cases.md` (TC-WF-*, TC-STD-* categories)
- [ ] 6.2 Append `WORKFLOW_INVALID`, `WORKFLOW_NOT_FOUND`, `STANDARD_INVALID`, `STANDARD_NOT_FOUND` to `pkg/errcode`
- [ ] 6.3 Implement workflow YAML parser in `core/internal/workflow`: schema validation (required fields, `kind` enum, reject unknown top-level keys), reserved-name check, case-insensitive duplicate warning
- [ ] 6.4 Implement colon-namespaced naming resolution for nested workflow files (lowercased, joined with `:`)
- [ ] 6.5 Implement `tai workflow list` and `tai workflow run <name>` — markdown plan emitter with Required-tools, Steps, Failure-mode sections
- [ ] 6.6 Implement standards loader in `core/internal/standards`: frontmatter parsing, `(missing description in frontmatter)` fallback, body emission with frontmatter stripped
- [ ] 6.7 Implement `tai standards list` and `tai standards load <name>` — colon-namespaced lowercased addressing, reserved-name validation
- [ ] 6.8 Verify with e2e tests that neither workflows nor standards are ever written to a target during `tai sync`

## 7. Phase 3 — Content surfaces: built-in slash commands

- [ ] 7.1 Add BDD cases for `install-commands` to `core/test-cases.md` (TC-IC-* category)
- [ ] 7.2 Embed TAI's bundled built-in slash commands (currently a starter set documenting workflow/standards invocation patterns) into the core binary
- [ ] 7.3 Implement `tai install-commands`: iterate over configured targets, write bundled commands into `<root>/<commands>/tai/`, skip falsy-commands targets with stderr warning
- [ ] 7.4 Implement re-run idempotency: replace existing files in `tai/` subdirectory, remove stale built-ins that the new binary no longer ships
- [ ] 7.5 Verify content outside `<root>/<commands>/tai/` is untouched by re-run tests

## 8. Phase 4 — Plugin host

- [ ] 8.1 Add BDD cases for `plugin-host` to `core/test-cases.md` (TC-PLG-* category)
- [ ] 8.2 Append `PLUGIN_UNKNOWN`, `PLUGIN_NAME_RESERVED`, `PLUGIN_ASSET_NAMING`, `PLUGIN_FETCH_UNAUTHORIZED`, `PLUGIN_FETCH_FAILED` to `pkg/errcode`
- [ ] 8.3 Create `pkg/taiplugin` SDK: parse `TAI_CLONE_DIR`, `TAI_TARGETS` (JSON), `TAI_DATA_DIR` into a typed `Context`; re-export `errcode` and `cliout` for plugin author ergonomics
- [ ] 8.4 Define the built-in first-party registry in `core/internal/plugins/registry.go` as a `map[string]Source` — start with `triage` → this repo's GitHub Releases
- [ ] 8.5 Implement plugin install: registry/explicit-source resolution, GitHub Releases asset fetch matching `tai-plugin-<name>-<os>-<arch>`, opportunistic `GITHUB_TOKEN`, write binary + assets/ to `<TAI_DATA_DIR>/plugins/<name>/`, validate `tai-<name>-` prefix on skills/agents, sync assets into every target's namespace, record in `<TAI_DATA_DIR>/state/plugins.json`
- [ ] 8.6 Implement plugin update: re-fetch latest from recorded source, wipe namespace in each target, re-copy, update state
- [ ] 8.7 Implement plugin remove: wipe target namespace and `<TAI_DATA_DIR>/plugins/<name>/` (preserve `state/` subdir), warn user about retained data
- [ ] 8.8 Implement `tai plugins list` — table output from state file
- [ ] 8.9 Implement subprocess invocation for `tai <plugin-name> <args>`: lookup, exec, env-var contract, passthrough stdin/stdout/stderr/exit; emit `UNKNOWN_SUBCOMMAND` for unresolved names
- [ ] 8.10 Implement `plugins.yml` additive auto-install at start of `tai sync`
- [ ] 8.11 Document the plugin wire contract (env vars, asset namespacing, error template expectation) AND the "to add a first-party plugin: register in `core/internal/plugins/registry.go` + cut release" workflow in `CLAUDE.md`. This MUST happen in this phase (not Phase 7) — Phase 6 (Triage migration) wires the first plugin and would otherwise set precedent with no documented contract. README sections may still be deferred to Phase 7 but `CLAUDE.md` updates land here

## 9. Phase 5 — Update banner

- [ ] 9.1 Add BDD cases for `update-banner` to `core/test-cases.md` (TC-UB-* category)
- [ ] 9.2 Implement the background check goroutine that refreshes `<TAI_DATA_DIR>/state/update-check.json` when older than `update-check-interval` (default 6h, configurable, `0` disables)
- [ ] 9.3 Implement the once-per-day banner gated by `last-banner-date` field; aggregates pending updates across TAI itself, installed plugins, and source repo
- [ ] 9.4 Verify banner is on stderr only, prefixed `[tai]`, at most 4 lines, names exact commands the user runs to update
- [ ] 9.5 Verify `tai update` exits with `UNKNOWN_SUBCOMMAND` (no self-update verb)
- [ ] 9.6 Verify no banner fires twice on the same calendar day

## 10. Phase 6 — Triage plugin migration

- [ ] 10.1 Add new TC-IDs to `plugins/triage/test-cases.md` for any new behavior introduced by the migration (most existing cases carry forward verbatim)
- [ ] 10.2 Build `plugins/triage/cmd/triage/main.go` as a standalone binary entrypoint consuming `TAI_*` env vars via `pkg/taiplugin`
- [ ] 10.3 Re-namespace existing Triage subcommands so they are exposed as `tai triage <verb>` via TAI's subprocess invocation (the verbs themselves — `import`, `accept`, `list`, etc. — stay the same)
- [ ] 10.4 Move existing Triage AI bundled commands (the `cmdframework`-managed assets) into `plugins/triage/assets/commands/` and verify they install into `<target>/<commands>/tai-triage/`
- [ ] 10.5 Re-point every Triage AI end-to-end test in `plugins/triage/internal/cmdtest` to invoke via the new `tai triage <verb>` route, exercising the subprocess wiring
- [ ] 10.6 Verify the Triage plugin's SQLite database lives at `<TAI_DATA_DIR>/plugins/triage/state/` and is created lazily
- [ ] 10.7 Verify Triage plugin assets named `tai-triage-*` (skills/agents) pass install-time namespace validation; rename any non-conformant assets in `plugins/triage/assets/`
- [ ] 10.8 Add `triage` to the first-party registry entry in `core/internal/plugins/registry.go`

## 11. Phase 7 — Documentation and release pipeline

- [ ] 11.1 Rewrite README.md: TAI origin story (Triage AI → TAI), problem statement, install instructions, source-repo concept, plugin authoring overview
- [ ] 11.2 Finalize CLAUDE.md: review the structural edits made in Phase 0 (task 1.8) and the plugin-authoring section landed in Phase 4 (task 8.11) for end-to-end coherence; tighten or expand wording now that every phase has shipped. No net-new structural content should land here — the load-bearing edits already happened
- [ ] 11.3 Verify CONTEXT.md reflects current vocabulary; add nesting marker (CONTEXT-MAP.md) only if splits are introduced in this proposal
- [ ] 11.4 Extend `.github/workflows/` (or equivalent CI) to build matrix: `tai` from `core/cmd/tai/` + `triage` from `plugins/triage/cmd/triage/` across linux/darwin/windows × amd64/arm64; attach as release assets named `tai-<os>-<arch>` and `tai-plugin-triage-<os>-<arch>`
- [ ] 11.5 Document the wire-level plugin contract (env vars, error-template expectations, asset-naming rules) as a top-level section in README.md and link from CLAUDE.md

## 12. Cross-cutting verification

- [ ] 12.1 Full green run: `go test ./... && go vet ./... && gofmt -l . && go test -race ./...`
- [ ] 12.2 Verify the footer-regex invariant test from `pkg/cliout` covers every new error code added in this change
- [ ] 12.3 Spot-check that every TC-ID added to `core/test-cases.md` and `plugins/triage/test-cases.md` is referenced by at least one test (`grep -r TC-<ID> ...`)
- [ ] 12.4 Verify the spec deltas in `openspec/changes/pivot-to-ai-as-code/specs/**/*.md` archive cleanly: `openspec status --change pivot-to-ai-as-code` reports `isComplete: true` once all checkboxes are checked
- [ ] 12.5 Update `openspec/changes/archive/` with the proposal after merge per the CLAUDE.md archival rule
