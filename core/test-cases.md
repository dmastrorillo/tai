# test-cases.md — tai core behavioural specification

This file is the authoritative, human-readable specification of how the
`tai` core CLI behaves. It holds BDD-style Given / When / Then scenarios
covering happy paths, edge cases, and known historical regressions.
**It is the contract; the code is downstream.**

The flow is: **OpenSpec proposal → BDD cases here → tests → production
code → observed CLI behaviour.** A change is "real" only after it appears
as a Given / When / Then below, is exercised by a test that names its
TC-ID, and is implemented behind that test.

Framework behaviour (error contract, panic recovery, error-code
taxonomy) lives in [`pkg/test-cases.md`](../pkg/test-cases.md);
triage-plugin behaviour lives in
[`plugins/triage/test-cases.md`](../plugins/triage/test-cases.md). See
[`CLAUDE.md`](../CLAUDE.md) for the full pipeline and ID-scheme rules.

---

## Categories

Each case has an ID of the form `TC-<CATEGORY>-<NUMBER>`. Categories are
short, stable codes; numbers increment within each category starting at
`001`, zero-padded to 3 digits, and remain globally unique across
components. **Never renumber existing IDs.**

| Code | Scope |
|------|-------|
| [`CMD`](#cmd--command-wiring--meta-verbs) | Top-level command wiring, help, version, unknown subcommands |
| [`CONF`](#conf--config-file-management) | YAML config: path resolution, lazy creation, schema, `tai config` CLI surface |
| [`CLI`](#cli--stdoutstderr-discipline) | Stdout-vs-stderr discipline, TTY-gated decoration |
| [`SYNC`](#sync--source-repo-sync) | `tai sync`: clone, fetch, overwrite detection, manifest, prune, background poll |
| [`INIT`](#init--repo-scaffold) | `tai repo init <path>` scaffold, git init, next-steps block |
| [`WF`](#wf--workflows) | `tai workflow list/run`: YAML schema, colon-namespaced naming, markdown plan emitter |
| [`STD`](#std--standards) | `tai standards list/load`: markdown + frontmatter, colon-namespaced addressing |
| [`IC`](#ic--install-commands) | `tai install-commands`: bundled slash-command install into `<target>/<commands>/tai/`, falsy skip, idempotent re-run, stale removal |
| [`PLG`](#plg--plugin-host) | `tai plugins` and the subprocess invocation: registry lookup, install/update/remove/list, asset namespacing, env-var contract, `plugins.yml` auto-install on sync |
| [`UB`](#ub--update-banner) | Background update-check refresh of TAI/plugin/source-repo versions, once-per-day stderr banner, `tai update` non-verb |
| [`REL`](#rel--release-cycle--prefix-aware-lookups) | Release-pipeline contracts owned by core: plugin asset-filename pin, prefix-aware "latest" lookup algorithm for plugin streams, banner plugin-row behaviour under prefixed tags |

(Cases originally numbered TC-CMD-003 through TC-CMD-007 cover the
bundled-command-framework parser used by the Triage plugin and live in
[`plugins/triage/test-cases.md`](../plugins/triage/test-cases.md). The
TC-ERR-* category covering the foundation error contract lives in
[`pkg/test-cases.md`](../pkg/test-cases.md) — the writer is in
`pkg/cliout` and the panic-recovery wrapper is in `pkg/cliexec`. TC-IDs
remain globally unique across components — never renumbered when a
section moves.)

---

## CMD — command wiring & meta-verbs

### TC-CMD-001 — `tai --version` prints the version string

- **Given** tai is invoked with no other arguments,
- **When** the user runs `tai --version`,
- **Then** stdout contains the literal substring `tai version <X>` where
  `<X>` is the build's version string (`dev` for local builds, `v0.x.y`
  for tagged releases),
- **And** stderr is empty,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/root_test.go` →
`TestVersion_TCCMD001_prints_version_string` (against the core binary's
root). The triage plugin's separate root carries an equivalent test at
`plugins/triage/internal/cmd/root_test.go` for the plugin binary.

### TC-CMD-002 — unknown subcommand / flag exits with `UNKNOWN_SUBCOMMAND`

- **Given** the root command has no subcommand or flag matching the
  user's input,
- **When** the user runs `tai --bogus-flag`, `tai bogus`, or any other
  unrecognised token,
- **Then** stderr contains the standard error template (Error line,
  "What to do:" block with a `tai --help` pointer, and the footer
  `[exit 1: UNKNOWN_SUBCOMMAND]`),
- **And** the exit code is `1`.

Exercised by `core/internal/cmd/root_test.go` →
`TestRoot_TCCMD002_unknown_flag` (flag form) and
`TestRoot_TCCMD002_unknown_positional` (positional form), both against
the core binary's root. The triage plugin's root carries equivalent
tests at `plugins/triage/internal/cmd/root_error_test.go` for the
plugin binary.

### TC-CMD-008 — `tai --help` outside a git repository exits 0

- **Given** the current directory is not inside any git repository,
- **When** the user runs `tai --help`,
- **Then** the CLI prints help to stdout (containing `tai`),
- **And** the exit code is `0`,
- **And** the CLI does not invoke the repo resolver (the absence of
  any `REPO_NOT_FOUND` signal in stderr is the user-visible evidence).

Exercised by `core/internal/cmd/root_test.go` →
`TestHelp_TCCMD008_outside_git_repo` (against the core binary's root —
the core root has no repo resolver, so cwd is irrelevant; the test
verifies stderr does not leak any REPO_NOT_FOUND signal). The triage
plugin's root, which DOES wire a repo resolver, carries the analogous
test at `plugins/triage/internal/cmd/repo_test.go` →
`TestHelp_TCCMD008_outside_git_repo` and
`TestVersion_TCCMD001_outside_git_repo` for the plugin binary.

<!-- Add new CMD cases here as their proposals land. -->

---

## CONF — config file management

The `tai config` family manages a YAML config file resolved per
`openspec/specs/config/spec.md`. The data-directory-resolution
cases under `TC-CFG-*` (in `plugins/triage/test-cases.md`) cover a
separate concept; do not conflate the two.

### TC-CONF-001 — Default config path on Linux with no overrides

- **Given** `$TAI_CONFIG` and `$XDG_CONFIG_HOME` are both unset and `$HOME` is `/tmp/fake-home`,
- **When** the config-loader resolves the path,
- **Then** the resolved path is `/tmp/fake-home/.config/tai/config.yml`.

Exercised by `core/internal/config/config_test.go` →
`TestResolve_TCCONF001_default_linux_path`.

### TC-CONF-002 — `TAI_CONFIG` wins over `XDG_CONFIG_HOME`

- **Given** `$TAI_CONFIG` is `/tmp/explicit/config.yml` and `$XDG_CONFIG_HOME` is also set,
- **When** the loader resolves the path,
- **Then** the resolved path is `/tmp/explicit/config.yml` (used verbatim — no `tai/` suffix appended).

Exercised by `core/internal/config/config_test.go` →
`TestResolve_TCCONF002_tai_config_overrides`.

### TC-CONF-003 — First invocation does not create the config file

- **Given** no config file exists at the resolved path,
- **When** the user runs `tai --help` or `tai --version`,
- **Then** the command succeeds with exit `0`,
- **And** no file or directory is created at the resolved path.

Exercised by `core/internal/cmd/config_test.go` →
`TestRoot_TCCONF003_help_does_not_create_config` and
`TestRoot_TCCONF003_version_does_not_create_config`.

### TC-CONF-004 — Lazy creation on first write

- **Given** no config file exists at the resolved path,
- **When** the user runs `tai config target add /tmp/example`,
- **Then** the resolved path's parent directory is created if needed,
- **And** a new config file is written containing the new target,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfig_TCCONF004_lazy_create_on_first_write`.

### TC-CONF-005 — Sub-path defaults

- **Given** a target with `root: ~/.claude` and no explicit sub-paths,
- **When** the config is loaded and effective paths are resolved,
- **Then** the effective `skills` is `~/.claude/skills`, effective `commands` is `~/.claude/commands`, and effective `agents` is `~/.claude/agents`.

Exercised by `core/internal/config/config_test.go` →
`TestTarget_TCCONF005_effective_subpath_defaults`.

### TC-CONF-006 — `repo-url` rejects local paths and `file://`

- **Given** the user runs `tai config set repo-url file:///tmp/repo`,
- **When** the command executes,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: CONFIG_INVALID_REPO_URL]`,
- **And** the existing config (if any) is unchanged.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigSet_TCCONF006_rejects_file_url` and
`TestConfigSet_TCCONF006_rejects_local_path`.

### TC-CONF-007 — Target with every sub-path falsy is rejected

- **Given** a config containing a target with `skills: ""`, `commands: ""`, `agents: ""`,
- **When** the config is loaded,
- **Then** loading fails with `CONFIG_INVALID`,
- **And** the error message names the offending target by its `root`.

Exercised by `core/internal/config/config_test.go` →
`TestLoad_TCCONF007_all_falsy_subpaths_rejected`.

### TC-CONF-008 — `tai config show` prints YAML with an existing config

- **Given** a populated config file at the resolved path with `repo-url` and one target,
- **When** the user runs `tai config show`,
- **Then** stdout contains the YAML representation of the config (the `repo-url` value and the target's `root` are both substring-visible),
- **And** stderr is empty,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigShow_TCCONF008_prints_yaml`.

### TC-CONF-009 — `tai config show` with no config prints informational message

- **Given** no config file exists at the resolved path,
- **When** the user runs `tai config show`,
- **Then** stdout names the resolved path and the next-step commands (`tai config target add`, `tai config edit`),
- **And** the exit code is `0` (absence of a config is not an error).

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigShow_TCCONF009_no_config_message`.

### TC-CONF-010 — `tai config edit` creates a commented template on first call

- **Given** no config file exists and `$EDITOR` is a fake recording binary,
- **When** the user runs `tai config edit`,
- **Then** the resolved config path now exists on disk,
- **And** the file contents include commented documentation for every supported top-level key (`repo-url`, `targets`, `update-check-interval`),
- **And** the editor's recorded argv contains the resolved config path.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigEdit_TCCONF010_creates_template_and_opens_editor`.

### TC-CONF-011 — `tai config edit` round-trips an existing config unchanged

- **Given** a populated config file exists and `$EDITOR` is a no-op binary that exits 0,
- **When** the user runs `tai config edit`,
- **Then** the config file's bytes are byte-for-byte identical before and after the command.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigEdit_TCCONF011_roundtrip_unchanged`.

### TC-CONF-012 — `tai config edit` without `$EDITOR` errors

- **Given** `$EDITOR` is unset,
- **When** the user runs `tai config edit`,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: CONFIG_EDITOR_UNSET]`,
- **And** the config file is not created or modified.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigEdit_TCCONF012_no_editor`.

### TC-CONF-013 — `tai config set` updates a scalar top-level key

- **Given** a populated config file with no `repo-url`,
- **When** the user runs `tai config set repo-url git@github.com:acme/repo.git`,
- **Then** the file now contains `repo-url: git@github.com:acme/repo.git`,
- **And** any other top-level keys (e.g. `targets`) are preserved unchanged.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigSet_TCCONF013_updates_repo_url`.

### TC-CONF-014 — `tai config set` rejects nested/array keys

- **Given** any config-file state,
- **When** the user runs `tai config set targets[0].root ~/.claude`,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: CONFIG_KEY_NOT_SCRIPTABLE]`,
- **And** the config file is unchanged.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigSet_TCCONF014_rejects_nested_key`.

### TC-CONF-015 — `tai config target add` appends a new target

- **Given** a config file with zero targets,
- **When** the user runs `tai config target add ~/.claude --skills custom-skills`,
- **Then** the config file gains a target with `root: ~/.claude` and `skills: custom-skills`,
- **And** `commands` and `agents` are absent from the YAML (defaults applied at sync time).

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigTargetAdd_TCCONF015_appends_new`.

### TC-CONF-016 — `tai config target add` rejects duplicate root

- **Given** a config file containing a target with `root: ~/.claude`,
- **When** the user runs `tai config target add ~/.claude`,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: CONFIG_DUPLICATE_TARGET]`,
- **And** the existing target is preserved unchanged.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigTargetAdd_TCCONF016_duplicate_rejected`.

### TC-CONF-017 — `tai config target list` with no targets

- **Given** a config file (or no file at all) with zero targets,
- **When** the user runs `tai config target list`,
- **Then** stdout contains the literal line `(no targets configured)`,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigTargetList_TCCONF017_empty`.

### TC-CONF-018 — `tai config target list` renders a table

- **Given** a config with two targets,
- **When** the user runs `tai config target list`,
- **Then** stdout contains a header row with `root`, `skills`, `commands`, `agents`,
- **And** each target's `root` value appears on its own data row,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigTargetList_TCCONF018_renders_table`.

### TC-CONF-019 — `tai config target remove` deletes an existing target

- **Given** a config with a target rooted at `~/.claude` and another at `~/.opencode`,
- **When** the user runs `tai config target remove ~/.claude`,
- **Then** the config file no longer contains the `~/.claude` target,
- **And** the `~/.opencode` target is preserved unchanged.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigTargetRemove_TCCONF019_removes_existing`.

### TC-CONF-020 — `tai config target remove` on missing target errors

- **Given** a config without a target rooted at `~/.nope`,
- **When** the user runs `tai config target remove ~/.nope`,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: CONFIG_TARGET_NOT_FOUND]`,
- **And** the config file is unchanged.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigTargetRemove_TCCONF020_missing_errors`.

<!-- Add new CONF cases here as their proposals land. -->

---

## CLI — stdout/stderr discipline

This category locks the foundation's stdout-vs-stderr contract across
every command in core. Stdout is the data channel (parsed by AI agents
and shell pipelines); stderr is the conversation channel (errors,
prompts, warnings, the update banner). Phase 1 introduces the
TTY-detection helper that downstream emitters use to suppress ANSI/CR
when stdout is not a terminal — later phases (`tai sync` prompts, the
update banner) extend this with channel-specific assertions for their
own behaviour.

### TC-CLI-001 — TTY detection treats a non-`*os.File` writer as non-TTY

- **Given** a writer that is not an `*os.File` (e.g. a `*bytes.Buffer` used by tests),
- **When** `cliout.IsTTY(w)` is called,
- **Then** the returned bool is `false`.

Exercised by `pkg/cliout/tty_test.go` →
`TestIsTTY_TCCLI001_non_file_is_non_tty`.

### TC-CLI-002 — TTY detection treats a regular-file writer as non-TTY

- **Given** an `*os.File` opened against a regular file on disk,
- **When** `cliout.IsTTY(f)` is called,
- **Then** the returned bool is `false`.

Exercised by `pkg/cliout/tty_test.go` →
`TestIsTTY_TCCLI002_regular_file_is_non_tty`.

### TC-CLI-003 — `tai config target list` routes data to stdout, leaves stderr empty

- **Given** a config with one or more targets configured,
- **When** the user runs `tai config target list`,
- **Then** stdout receives the table (header + one row per target),
- **And** stderr is empty,
- **And** the exit code is `0`.

This is the first concrete channel-discipline anchor in core: the
spec's "stdout = data, stderr = conversation" rule needs a worked
example tied to a real command, not just the in-isolation TTY helper
tests. Later commands (sync prompts, the update banner) extend the
channel discipline with their own TC-CLI-* cases.

Exercised by `core/internal/cmd/config_test.go` →
`TestConfigTargetList_TCCLI003_channel_discipline`.

<!-- Add new CLI cases here as their proposals land. -->

---

## SYNC — source-repo sync

`tai sync` clones the configured `repo-url` into `<TAI_DATA_DIR>/source/`,
fetches updates, and copies assets into configured targets with M1
existence-based overwrite detection. Background update-polling is wired
into every invocation. Spec:
`openspec/specs/repo-sync/spec.md`.

Data-directory resolution (`<TAI_DATA_DIR>` precedence, lazy creation,
unwritable handling) is owned by `pkg/datadir` and pinned by the
`TC-CFG-*` cases in [`pkg/test-cases.md`](../pkg/test-cases.md);
`DATA_DIR_UNWRITABLE` failures surface from that contract, not from
the `TC-SYNC-*` cases below.

### TC-SYNC-001 — First sync creates the clone

- **Given** a configured `repo-url` pointing at a reachable remote and no clone yet at `<TAI_DATA_DIR>/source/`,
- **When** the user runs `tai sync`,
- **Then** `<TAI_DATA_DIR>/source/.git/` exists after the command completes.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC001_first_sync_creates_clone`.

### TC-SYNC-002 — Subsequent sync reuses the clone

- **Given** a clone already exists at `<TAI_DATA_DIR>/source/`,
- **When** the user runs `tai sync` a second time,
- **Then** the existing `.git/` directory's inode is unchanged (no re-clone),
- **And** the workspace is fast-forwarded to the upstream tip.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC002_subsequent_sync_reuses_clone`.

### TC-SYNC-003 — Fetch failure surfaces cache-fallback warning

- **Given** a clone exists and the network is unreachable,
- **When** the user runs `tai sync`,
- **Then** stderr contains a one-line warning naming "fetch failed" and the timestamp of the last successful fetch,
- **And** the sync proceeds against the cached clone,
- **And** the exit code is `0` (the fetch failure does not abort the sync).

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC003_fetch_failure_warning`.

### TC-SYNC-004 — Fresh sync to empty target writes every source file

- **Given** the source repo has 3 skill files and the target has no existing files,
- **When** the user runs `tai sync`,
- **Then** all 3 skill files exist at `<target>/<skills>/<name>`,
- **And** no overwrite prompt is shown.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC004_fresh_writes_all_files`.

### TC-SYNC-005 — Sync with one overwrite emits a single prompt

- **Given** the source repo has a skill `foo.md` and the target already has a file at `<target>/<skills>/foo.md`,
- **When** the user runs `tai sync`,
- **Then** stderr contains a prompt listing `foo.md` under skills as "will be overwritten",
- **And** stdin is read for a `y` or `N` response.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC005_single_overwrite_prompt`.

### TC-SYNC-006 — Sync batches multiple overwrites into one prompt

- **Given** the source has 5 skills, 2 commands, and 1 agent that all already exist at the target,
- **When** the user runs `tai sync`,
- **Then** TAI emits ONE prompt grouping the 8 paths under their three categories,
- **And** does not prompt individually per file.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC006_batched_overwrite_prompt`.

### TC-SYNC-007 — `-y` bypasses the overwrite prompt

- **Given** at least one overwrite is pending,
- **When** the user runs `tai sync -y`,
- **Then** no prompt is shown,
- **And** the overwritten files are listed on stderr after writing for visibility,
- **And** the destination files are overwritten with the source bytes.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC007_dash_y_bypasses_prompt`.

### TC-SYNC-008 — User answering `N` cancels the sync

- **Given** at least one overwrite is pending and stdin yields `N\n`,
- **When** the user runs `tai sync` (no `-y`),
- **Then** the exit code is `0` (cancellation is not an error),
- **And** no files are written to the target,
- **And** stderr contains a cancellation message.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC008_user_rejection_cancels`.

### TC-SYNC-009 — First sync creates the per-target manifest

- **Given** the source has 3 skills, 1 command, 0 agents and no manifest exists for the target,
- **When** the user runs `tai sync`,
- **Then** `<TAI_DATA_DIR>/manifests/<sha256-of-target-root>.json` exists,
- **And** contains the 4 written relative paths.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC009_first_sync_creates_manifest`.

### TC-SYNC-010 — Subsequent sync extends the manifest

- **Given** a manifest with 4 paths and the source has gained 1 new skill file,
- **When** the user runs `tai sync`,
- **Then** the manifest now contains 5 paths (the union of the original 4 and the new one).

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC010_subsequent_sync_extends_manifest`.

### TC-SYNC-011 — `--prune` deletes orphans on confirm

- **Given** a manifest references a previously-synced skill that has since been removed from the source,
- **When** the user runs `tai sync --prune -y`,
- **Then** the orphan file no longer exists in the target,
- **And** the manifest no longer contains the orphan's path.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC011_prune_deletes_orphans`.

### TC-SYNC-012 — Sync without `--prune` surfaces an orphan-count line

- **Given** one previously-synced file has been removed from source and the user runs `tai sync` (no `--prune`),
- **When** the sync completes,
- **Then** the file still exists at the target,
- **And** stderr contains a line matching `1 orphan pending`.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC012_orphan_count_summary`.

### TC-SYNC-013 — `tai sync` requires both `repo-url` and `targets`

- **Given** the config has `repo-url` set but `targets` empty,
- **When** the user runs `tai sync`,
- **Then** the exit code is `2`,
- **And** stderr's footer is `[exit 2: TAI_NOT_CONFIGURED]`,
- **And** the "what to do" bullets name `tai config target add` as the resolution.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSYNC013_requires_both_config_fields`.

### TC-SYNC-014 — Background poll refreshes cache when stale

- **Given** `<TAI_DATA_DIR>/state/update-check.json` has a timestamp older than `update-check-interval` and the source repo is reachable,
- **When** the user runs any TAI command,
- **Then** the foreground command completes without blocking on the poll,
- **And** within a short bounded wait after exit, the cache file's timestamp is newer than the original.

Exercised by `core/internal/cmd/sync_test.go` →
`TestUpdatePoll_TCSYNC014_stale_cache_refreshed`.

### TC-SYNC-015 — Background poll leaves cache untouched when fresh

- **Given** the cache file's timestamp is within `update-check-interval`,
- **When** the user runs any TAI command,
- **Then** the cache file is byte-identical before and after the command exits.

Exercised by `core/internal/cmd/sync_test.go` →
`TestUpdatePoll_TCSYNC015_fresh_cache_untouched`.

### TC-SYNC-016 — Background poll error is silently absorbed

- **Given** the cache file is stale and the source repo is unreachable,
- **When** the user runs any TAI command,
- **Then** the foreground command's stdout and stderr contain no warning attributable to the background poll,
- **And** the cache file is byte-identical before and after the command exits.

Exercised by `core/internal/cmd/sync_test.go` →
`TestUpdatePoll_TCSYNC016_poll_error_silent`.

### TC-SYNC-017 — Background poll is disabled when `update-check-interval = 0`

- **Given** the config has `update-check-interval: 0`,
- **When** the user runs any TAI command (even with a stale cache file present),
- **Then** the cache file is byte-identical before and after the command exits.

Exercised by `core/internal/cmd/sync_test.go` →
`TestUpdatePoll_TCSYNC017_disabled_skips_poll`.

### TC-SYNC-018 — Background poll does not prompt for credentials

- **Given** the configured `repo-url` is an HTTPS URL pointing at a private repository,
- **And** no credential helper is configured for that host,
- **When** the user runs any TAI command and the background poll fires,
- **Then** no `Username for ...` (or analogous) prompt is written to any output stream,
- **And** stdin is not read by the background `git` process (the user's keystrokes intended for the foreground command are not consumed),
- **And** the cache file at `<TAI_DATA_DIR>/state/update-check.json` is left unchanged (the poll fails silently per the existing absorption rule, TC-SYNC-016).

Exercised by `core/internal/sync/poll_creds_test.go` →
`TestUpdatePoll_TCSYNC018_no_creds_prompt`.

### TC-SYNC-019 — Foreground sync still prompts for credentials when interactive auth is needed

- **Given** the configured `repo-url` is an HTTPS URL pointing at a private repository,
- **And** no credential helper is configured for that host,
- **When** the user runs `tai sync` in an interactive TTY,
- **Then** the foreground `git fetch` (or `git clone` on first sync) MAY prompt for credentials normally,
- **And** the foreground process does NOT inherit the background poll's non-interactive env vars (`GIT_TERMINAL_PROMPT`, `GIT_ASKPASS`, `GCM_INTERACTIVE`).

Exercised by `core/internal/sync/poll_creds_test.go` →
`TestSyncForeground_TCSYNC019_keeps_interactive_creds`.

<!-- Add new SYNC cases here as their proposals land. -->

---

## INIT — repo scaffold

`tai repo init <path>` writes a templated source-repo scaffold,
auto-initialises a git repo, and prints next-step commands. Spec:
`openspec/specs/repo-init/spec.md`.

### TC-INIT-001 — Fresh directory scaffold writes every required file

- **Given** `<path>` does not exist yet,
- **When** the user runs `tai repo init <path>`,
- **Then** `<path>` is created,
- **And** the following files exist with non-empty content: `<path>/README.md`, `<path>/skills/README.md`, `<path>/commands/README.md`, `<path>/agents/README.md`, `<path>/workflows/README.md`, `<path>/standards/README.md`, `<path>/.gitignore`,
- **And** `<path>/plugins.yml` exists (may be empty list with a commented example).

Exercised by `core/internal/cmd/repo_init_test.go` →
`TestRepoInit_TCINIT001_fresh_directory`.

### TC-INIT-002 — Scaffold into an existing empty directory succeeds

- **Given** `<path>` exists and contains zero files,
- **When** the user runs `tai repo init <path>`,
- **Then** the scaffold is written into the existing directory,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/repo_init_test.go` →
`TestRepoInit_TCINIT002_existing_empty_dir`.

### TC-INIT-003 — Non-empty target is rejected

- **Given** `<path>` contains at least one file or subdirectory,
- **When** the user runs `tai repo init <path>`,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: REPO_INIT_TARGET_NOT_EMPTY]`,
- **And** no files in `<path>` are created or modified.

Exercised by `core/internal/cmd/repo_init_test.go` →
`TestRepoInit_TCINIT003_non_empty_target_rejected`.

### TC-INIT-004 — Scaffolded READMEs document their conventions

- **Given** a successful `tai repo init <path>`,
- **When** the per-folder READMEs are inspected,
- **Then** `<path>/skills/README.md` contains the substring `tai-<plugin>-` (the namespacing rule),
- **And** `<path>/workflows/README.md` contains the substring `description:` (a YAML schema field),
- **And** `<path>/standards/README.md` contains the substring `:` (colon-namespaced addressing),
- **And** `<path>/plugins.yml` contains at least one line beginning with `#` (the commented example).

Exercised by `core/internal/cmd/repo_init_test.go` →
`TestRepoInit_TCINIT004_readme_content`.

### TC-INIT-005 — Successful init creates a git repo with the standard initial commit

- **Given** `git` is on PATH and the scaffold succeeds,
- **When** the user runs `tai repo init <path>`,
- **Then** `<path>/.git/` exists,
- **And** `git -C <path> log -1 --format=%s` outputs `Initial TAI source-repo scaffold`.

Exercised by `core/internal/cmd/repo_init_test.go` →
`TestRepoInit_TCINIT005_git_init_and_commit`.

### TC-INIT-006 — Missing `git` surfaces REPO_INIT_GIT_UNAVAILABLE

- **Given** `git` is not on PATH,
- **When** the user runs `tai repo init <path>`,
- **Then** the scaffold files are written to disk,
- **And** the exit code is `3`,
- **And** stderr's footer is `[exit 3: REPO_INIT_GIT_UNAVAILABLE]`.

Exercised by `core/internal/cmd/repo_init_test.go` →
`TestRepoInit_TCINIT006_git_unavailable`.

### TC-INIT-007 — Next-steps block on stdout

- **Given** a successful `tai repo init /tmp/my-repo`,
- **When** the command exits,
- **Then** stdout contains the literal phrase `Next steps:`,
- **And** stdout contains a `git remote add origin` example line,
- **And** stdout contains a `tai config set repo-url` example line.

Exercised by `core/internal/cmd/repo_init_test.go` →
`TestRepoInit_TCINIT007_next_steps_block`.

### TC-INIT-008 — Local config is not modified by init

- **Given** the user has a populated config file at the resolved path,
- **When** the user runs `tai repo init <somewhere-else>`,
- **Then** the config file's bytes are identical before and after the command.

Exercised by `core/internal/cmd/repo_init_test.go` →
`TestRepoInit_TCINIT008_local_config_untouched`.

### TC-INIT-009 — Top-level README backlinks tai and explains the product

- **Given** a successful `tai repo init <path>`,
- **When** `<path>/README.md` is inspected,
- **Then** the body contains the substring `https://github.com/dmastrorillo/tai` (the upstream-project backlink),
- **And** the body contains an introductory sentence describing tai (e.g. the word `tai` plus a description like "CLI for sharing AI tooling" or similar),
- **And** the body does NOT contain the substring `docs.tai.sh` (no hallucinated documentation domain — that URL does not exist).

Exercised by `core/internal/cmd/repo_init_test.go` →
`TestRepoInit_TCINIT009_readme_backlinks_tai`.

<!-- Add new INIT cases here as their proposals land. -->

---

## WF — workflows

`tai workflow list/run` exposes YAML workflow files under
`<clone>/workflows/**/*.yml` to AI agents as markdown plans. Spec:
`openspec/specs/workflows/spec.md`.

### TC-WF-001 — Valid workflow loads successfully

- **Given** a workflow file at `<clone>/workflows/propose.yml` with a `description` and two `steps` entries whose `kind` values are `skill` and `command`,
- **When** the loader walks the workflows tree,
- **Then** the workflow is accepted with name `propose` and both steps preserved in order.

Exercised by `core/internal/workflow/workflow_test.go` →
`TestLoad_TCWF001_valid_workflow_accepted`.

### TC-WF-002 — `kind: agent` is rejected

- **Given** a workflow file with a step where `kind: agent`,
- **When** the loader walks the workflows tree,
- **Then** the call returns `*errcode.Error{Code: WORKFLOW_INVALID}`,
- **And** the message names the offending file and the offending step.

Exercised by `core/internal/workflow/workflow_test.go` →
`TestLoad_TCWF002_kind_agent_rejected`.

### TC-WF-003 — Unknown top-level key is rejected

- **Given** a workflow file containing a top-level key other than `description` or `steps` (e.g. `notes:`),
- **When** the loader walks the workflows tree,
- **Then** the call returns `*errcode.Error{Code: WORKFLOW_INVALID}`,
- **And** the message names the offending key.

Exercised by `core/internal/workflow/workflow_test.go` →
`TestLoad_TCWF003_unknown_top_level_key_rejected`.

### TC-WF-004 — Missing required field is rejected

- **Given** a workflow file missing `description` OR with a step missing `kind` or `name`,
- **When** the loader walks the workflows tree,
- **Then** the call returns `*errcode.Error{Code: WORKFLOW_INVALID}` and the message identifies what's missing.

Exercised by `core/internal/workflow/workflow_test.go` →
`TestLoad_TCWF004_missing_required_fields_rejected`.

### TC-WF-005 — Nested workflow resolves to colon-namespaced name

- **Given** a workflow file at `<clone>/workflows/release/cut-rc.yml`,
- **When** the loader walks the workflows tree,
- **Then** the workflow is addressable as `release:cut-rc` (all segments lowercased).

Exercised by `core/internal/workflow/workflow_test.go` →
`TestLoad_TCWF005_nested_colon_namespaced_name`.

### TC-WF-006 — Reserved name `list` is rejected

- **Given** a workflow file at `<clone>/workflows/list.yml`,
- **When** the loader walks the workflows tree,
- **Then** the call returns `*errcode.Error{Code: WORKFLOW_INVALID}` and the message names `list` as a reserved sub-verb.

Exercised by `core/internal/workflow/workflow_test.go` →
`TestLoad_TCWF006_reserved_name_list_rejected`.

### TC-WF-007 — Reserved name `run` is rejected

- **Given** a workflow file at `<clone>/workflows/run.yml`,
- **When** the loader walks the workflows tree,
- **Then** the call returns `*errcode.Error{Code: WORKFLOW_INVALID}` and the message names `run` as a reserved sub-verb.

Exercised by `core/internal/workflow/workflow_test.go` →
`TestLoad_TCWF007_reserved_name_run_rejected`.

### TC-WF-008 — Case-insensitive duplicate emits a warning, alphabetically earlier file wins

- **Given** workflow files `<clone>/workflows/Build.yml` and `<clone>/workflows/build.yml` both exist,
- **When** the loader walks the workflows tree,
- **Then** loading succeeds (a collision is a warning, not an error),
- **And** a warning is emitted to the loader's diagnostic sink naming both files,
- **And** the addressable workflow `build` resolves to the file whose source path is alphabetically earlier (`Build.yml`).

Exercised by `core/internal/workflow/workflow_test.go` →
`TestLoad_TCWF008_duplicate_warning_first_wins`.

### TC-WF-009 — `tai workflow list` prints workflows alphabetically

- **Given** the clone contains three workflows (`propose`, `release:cut-rc`, `verify`),
- **When** the user runs `tai workflow list`,
- **Then** stdout contains three lines (one per workflow),
- **And** the lines appear in alphabetical order by colon-name,
- **And** each line is of the form `<colon-name>  <description>` (or `<colon-name>  (missing description)` when none was declared),
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/workflow_test.go` →
`TestWorkflowList_TCWF009_prints_alphabetical`.

### TC-WF-010 — `tai workflow list` with no workflows prints `(no workflows)`

- **Given** the clone exists but `<clone>/workflows/` is empty (or absent),
- **When** the user runs `tai workflow list`,
- **Then** stdout contains the literal `(no workflows)` and the exit code is `0`.

Exercised at the CLI boundary by `core/internal/cmd/workflow_test.go` →
`TestWorkflowList_TCWF010_no_workflows`, with a loader-level anchor
at `core/internal/workflow/workflow_test.go` →
`TestLoad_empty_workflows_dir_returns_empty`.

### TC-WF-011 — `tai workflow run <name>` emits the markdown plan

- **Given** a workflow `propose` exists with two steps (kind `skill`, kind `command`),
- **When** the user runs `tai workflow run propose`,
- **Then** stdout starts with an H1 naming the workflow,
- **And** stdout contains the workflow's `description` as the first paragraph,
- **And** stdout contains a "Required tools" section listing both steps as bullets of the form `<kind>:  /<name>`,
- **And** stdout contains a "Steps" section listing the steps in declaration order, numbered,
- **And** stdout contains a "Failure mode" section instructing the AI to abort when a required tool is unavailable,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/workflow_test.go` →
`TestWorkflowRun_TCWF011_emits_markdown_plan`.

### TC-WF-012 — `tai workflow run` on a missing name exits with `WORKFLOW_NOT_FOUND`

- **Given** no workflow named `nope` exists,
- **When** the user runs `tai workflow run nope`,
- **Then** the exit code is `2`,
- **And** stderr's footer is `[exit 2: WORKFLOW_NOT_FOUND]`.

Exercised by `core/internal/cmd/workflow_test.go` →
`TestWorkflowRun_TCWF012_missing_workflow`.

### TC-WF-013 — `tai sync` never copies workflows into a target

- **Given** the source repo contains `workflows/propose.yml`,
- **When** the user runs `tai sync`,
- **Then** no file appears under any target directory (`<target>/<skills>/`, `<target>/<commands>/`, `<target>/<agents>/`, or any other path) whose source was the workflows tree.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCWF013_workflows_never_copied`.

<!-- Add new WF cases here as their proposals land. -->

---

## STD — standards

`tai standards list/load` exposes markdown standards under
`<clone>/standards/**/*.md` to AI agents on demand. Spec:
`openspec/specs/standards/spec.md`.

### TC-STD-001 — Standard with frontmatter description

- **Given** a standard at `<clone>/standards/SDLC.md` whose YAML frontmatter contains `description: Software development lifecycle`,
- **When** the loader walks the standards tree,
- **Then** the parsed standard `sdlc` has description `Software development lifecycle`.

Exercised by `core/internal/standards/standards_test.go` →
`TestLoad_TCSTD001_description_from_frontmatter`.

### TC-STD-002 — Standard without frontmatter falls back to default description

- **Given** a standard at `<clone>/standards/SDLC.md` with no frontmatter,
- **When** the loader walks the standards tree,
- **Then** the parsed standard's description is the literal string `(missing description in frontmatter)`.

Exercised by `core/internal/standards/standards_test.go` →
`TestLoad_TCSTD002_missing_frontmatter_fallback`.

### TC-STD-003 — Nested standard resolves to colon-namespaced lowercased name

- **Given** a standard at `<clone>/standards/devOps/security/best-practices.md`,
- **When** the loader walks the standards tree,
- **Then** the standard is addressable as `devops:security:best-practices`.

Exercised by `core/internal/standards/standards_test.go` →
`TestLoad_TCSTD003_nested_colon_namespaced_name`.

### TC-STD-004 — Reserved name `list` is rejected

- **Given** a standard at `<clone>/standards/list.md`,
- **When** the loader walks the standards tree,
- **Then** the call returns `*errcode.Error{Code: STANDARD_INVALID}` and the message names `list` as a reserved sub-verb.

Exercised by `core/internal/standards/standards_test.go` →
`TestLoad_TCSTD004_reserved_name_list_rejected`.

### TC-STD-005 — Reserved name `load` is rejected

- **Given** a standard at `<clone>/standards/load.md`,
- **When** the loader walks the standards tree,
- **Then** the call returns `*errcode.Error{Code: STANDARD_INVALID}` and the message names `load` as a reserved sub-verb.

Exercised by `core/internal/standards/standards_test.go` →
`TestLoad_TCSTD005_reserved_name_load_rejected`.

### TC-STD-006 — Case-insensitive collision emits a warning, alphabetically earlier file wins

- **Given** both `<clone>/standards/Foo.md` and `<clone>/standards/foo.md` exist,
- **When** the loader walks the standards tree,
- **Then** loading succeeds,
- **And** a warning is emitted naming both files,
- **And** the addressable standard `foo` resolves to the alphabetically-earlier source path (`Foo.md`).

Exercised by `core/internal/standards/standards_test.go` →
`TestLoad_TCSTD006_duplicate_warning_first_wins`.

### TC-STD-007 — `tai standards list` prints standards alphabetically

- **Given** the clone has `standards/SDLC.md` and `standards/devOps/security/best-practices.md`,
- **When** the user runs `tai standards list`,
- **Then** stdout contains two lines, one per standard, in alphabetical order by colon-name,
- **And** each line is of the form `<colon-name>  <description>`,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/standards_test.go` →
`TestStandardsList_TCSTD007_prints_alphabetical`.

### TC-STD-008 — `tai standards list` with no standards prints `(no standards)`

- **Given** the clone exists but `<clone>/standards/` is empty (or absent),
- **When** the user runs `tai standards list`,
- **Then** stdout contains the literal `(no standards)` and the exit code is `0`.

Exercised by `core/internal/cmd/standards_test.go` →
`TestStandardsList_TCSTD008_no_standards`.

### TC-STD-009 — `tai standards load <name>` prints body with frontmatter stripped

- **Given** a standard `<clone>/standards/SDLC.md` whose frontmatter declares a description and whose body is `# SDLC\n\nReview before merging.\n`,
- **When** the user runs `tai standards load sdlc`,
- **Then** stdout contains the body byte-for-byte after frontmatter removal,
- **And** stdout does NOT contain the `description:` frontmatter line,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/standards_test.go` →
`TestStandardsLoad_TCSTD009_prints_body`.

### TC-STD-010 — `tai standards load` on a missing name exits with `STANDARD_NOT_FOUND`

- **Given** no standard named `nonexistent` exists,
- **When** the user runs `tai standards load nonexistent`,
- **Then** the exit code is `2`,
- **And** stderr's footer is `[exit 2: STANDARD_NOT_FOUND]`.

Exercised by `core/internal/cmd/standards_test.go` →
`TestStandardsLoad_TCSTD010_missing_standard`.

### TC-STD-011 — `tai sync` never copies standards into a target

- **Given** the source repo contains `standards/SDLC.md`,
- **When** the user runs `tai sync`,
- **Then** no file appears under any target directory whose source was the standards tree.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCSTD011_standards_never_copied`.

<!-- Add new STD cases here as their proposals land. -->

---

## IC — install-commands

`tai install-commands` writes TAI's own bundled slash-command assets
into every configured target's `<commands>/tai/` subdirectory. The
subdirectory is wholly TAI-owned: re-runs overwrite freely within it
and remove files the running binary no longer bundles. Content
outside `<commands>/tai/` is never touched. Spec:
`openspec/specs/install-commands/spec.md`.

### TC-IC-001 — Single-target install writes every bundled file

- **Given** the config has one target with default sub-paths,
- **When** the user runs `tai install-commands`,
- **Then** every bundled built-in command appears as a file under
  `<target.root>/commands/tai/<name>.md`,
- **And** stdout contains a one-line summary of the form
  `installed <N> command(s) into 1 target` (no `(... stale built-ins
  removed)` parenthetical on a clean first run),
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/install_commands_test.go` →
`TestInstallCommands_TCIC001_single_target_writes_bundle`.

### TC-IC-002 — Multi-target install fans out to every target

- **Given** the config has two targets `<rootA>` and `<rootB>`,
- **When** the user runs `tai install-commands`,
- **Then** every bundled built-in command appears under both
  `<rootA>/commands/tai/` and `<rootB>/commands/tai/`,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/install_commands_test.go` →
`TestInstallCommands_TCIC002_multi_target_fan_out`.

### TC-IC-003 — No targets configured exits `TAI_NOT_CONFIGURED`

- **Given** the config has zero targets,
- **When** the user runs `tai install-commands`,
- **Then** the exit code is `2`,
- **And** stderr's footer is `[exit 2: TAI_NOT_CONFIGURED]`,
- **And** the "what to do" bullets name `tai config target add` as
  the resolution.

Exercised by `core/internal/cmd/install_commands_test.go` →
`TestInstallCommands_TCIC003_no_targets`.

### TC-IC-004 — Falsy commands sub-path skips that target with warning

- **Given** a target whose YAML config sets `commands: ""`,
- **When** the user runs `tai install-commands`,
- **Then** no file is written under that target's root,
- **And** stderr contains a warning naming the skipped target,
- **And** when every configured target was skipped this way, stdout
  contains a one-line summary of the form
  `all <N> target(s) skipped — nothing installed` (distinct from the
  install summary so a zero-count read does not masquerade as a
  successful no-op),
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/install_commands_test.go` →
`TestInstallCommands_TCIC004_falsy_commands_skipped`.

### TC-IC-005 — Re-run is idempotent within the `tai/` subdirectory

- **Given** a successful first run has written the bundled commands,
- **When** the user runs `tai install-commands` a second time,
- **Then** the second-run files are byte-identical to the first-run
  files for every still-bundled command,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/install_commands_test.go` →
`TestInstallCommands_TCIC005_rerun_idempotent`.

### TC-IC-006 — Stale built-ins (no longer bundled) are removed on re-run

- **Given** a previous install put `<target>/commands/tai/legacy.md`
  on disk, and the running binary no longer bundles `legacy.md`,
- **When** the user runs `tai install-commands`,
- **Then** `<target>/commands/tai/legacy.md` no longer exists,
- **And** every currently-bundled command is present,
- **And** stdout's summary line includes a
  `(<K> stale built-in(s) removed)` parenthetical naming the removal
  count.

Exercised by `core/internal/cmd/install_commands_test.go` →
`TestInstallCommands_TCIC006_stale_builtin_removed`.

### TC-IC-007 — Content outside `<commands>/tai/` is left untouched

- **Given** the user has authored content at
  `<target>/commands/my-own.md` (outside the `tai/` subdirectory),
- **When** the user runs `tai install-commands`,
- **Then** `<target>/commands/my-own.md` exists with its original
  bytes unchanged.

Exercised by `core/internal/cmd/install_commands_test.go` →
`TestInstallCommands_TCIC007_outside_tai_untouched`.

<!-- Add new IC cases here as their proposals land. -->

---

## PLG — plugin host

`tai plugins <install|update|remove|list>` manages first- and
third-party plugins, and the root command's subprocess hook routes
unknown verbs to installed plugins. Plugins are subprocess
executables under `<TAI_DATA_DIR>/plugins/<name>/`; their assets are
namespaced under `tai-<name>-*` (skills/agents) and
`<commands>/tai-<name>/` (commands). Spec:
`openspec/specs/plugin-host/spec.md`.

### TC-PLG-001 — Installed plugin layout on disk

- **Given** the user successfully installs the first-party `triage`
  plugin via `tai plugins install triage`,
- **When** the install completes,
- **Then** `<TAI_DATA_DIR>/plugins/triage/triage` (or `triage.exe` on
  Windows) exists and is executable,
- **And** `<TAI_DATA_DIR>/plugins/triage/assets/` exists as a
  directory.

Exercised by `core/internal/plugins/install_test.go` →
`TestInstall_TCPLG001_plugin_layout_on_disk`.

### TC-PLG-002 — Subprocess invocation passes through stdin/stdout/stderr/exit

- **Given** a plugin `triage` is installed and resolves to an
  executable that prints `out` to stdout, `err` to stderr, and exits
  `7`,
- **When** the user runs `tai triage foo`,
- **Then** stdout contains `out`,
- **And** stderr contains `err`,
- **And** the exit code is `7`,
- **And** the plugin received `foo` as its `argv[1]`.

Exercised by `core/internal/cmd/plugin_invoke_test.go` →
`TestPluginInvoke_TCPLG002_passthrough`.

### TC-PLG-003 — Unknown verb exits `UNKNOWN_SUBCOMMAND` with plugin-aware help

- **Given** no plugin named `nope` is installed,
- **When** the user runs `tai nope`,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: UNKNOWN_SUBCOMMAND]`,
- **And** the "what to do" bullets name `tai plugins list` and
  `tai plugins install <name>` as the resolution.

Exercised by `core/internal/cmd/plugin_invoke_test.go` →
`TestPluginInvoke_TCPLG003_unknown_verb`.

### TC-PLG-004 — Reserved-verb collision is rejected at install

- **Given** the user attempts `tai plugins install config`,
- **When** the install command runs (before any file is written),
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: PLUGIN_NAME_RESERVED]`,
- **And** no directory is created under `<TAI_DATA_DIR>/plugins/`.

Exercised by `core/internal/cmd/plugins_test.go` →
`TestPluginsInstall_TCPLG004_reserved_name_rejected`.

### TC-PLG-005 — Env-var contract is set on the subprocess

- **Given** a plugin `triage` is installed and the config has one
  target `~/.claude` with default sub-paths and a configured
  `repo-url`,
- **When** TAI invokes the plugin,
- **Then** the child process's `TAI_DATA_DIR` is the absolute data
  directory,
- **And** `TAI_CLONE_DIR` is the absolute path to
  `<TAI_DATA_DIR>/source/`,
- **And** `TAI_TARGETS` is a JSON array containing one object whose
  `root` field is `~/.claude` and whose `skills`, `commands`,
  `agents` fields are the effective defaulted paths.

Exercised by `core/internal/cmd/plugin_invoke_test.go` →
`TestPluginInvoke_TCPLG005_env_var_contract`.

### TC-PLG-006 — Skill/agent asset namespace prefix enforced at install

- **Given** a plugin `mytool` whose downloaded bundle includes
  `assets/skills/foo.md` (no `tai-mytool-` prefix),
- **When** the user runs `tai plugins install mytool --source ...`,
- **Then** the install fails with `PLUGIN_ASSET_NAMING`,
- **And** the error message names the offending file,
- **And** no files are left under any configured target's namespace
  for `mytool`.

Exercised by `core/internal/plugins/install_test.go` →
`TestInstall_TCPLG006_skill_namespace_enforced`.

### TC-PLG-007 — Commands routed into `<commands>/tai-<plugin>/`

- **Given** a plugin `triage` whose bundle includes
  `assets/commands/import.md`,
- **When** the install completes against a target `~/.claude`,
- **Then** the file lands at `~/.claude/commands/tai-triage/import.md`
  (regardless of the source filename).

Exercised by `core/internal/plugins/install_test.go` →
`TestInstall_TCPLG007_commands_routed_into_namespace`.

### TC-PLG-008 — Unknown plugin without `--source` fails with `PLUGIN_UNKNOWN`

- **Given** no built-in registry entry for `acme-custom` exists and
  the user invokes `tai plugins install acme-custom` (no `--source`),
- **When** the install runs,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: PLUGIN_UNKNOWN]`,
- **And** the "what to do" bullets suggest passing `--source`.

Exercised by `core/internal/cmd/plugins_test.go` →
`TestPluginsInstall_TCPLG008_unknown_plugin`.

### TC-PLG-009 — 401 surfaces `PLUGIN_FETCH_UNAUTHORIZED`

- **Given** the install fetcher receives a 401 (or 403) from the
  Releases host and `GITHUB_TOKEN` is unset,
- **When** the install runs,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: PLUGIN_FETCH_UNAUTHORIZED]`,
- **And** the "what to do" bullets name `GITHUB_TOKEN` as the
  resolution.

Exercised by `core/internal/plugins/install_test.go` →
`TestInstall_TCPLG009_401_surfaces_unauthorized`.

### TC-PLG-010 — Generic fetch failure surfaces `PLUGIN_FETCH_FAILED`

- **Given** the install fetcher receives a 5xx or a network error,
- **When** the install runs,
- **Then** the exit code is `3`,
- **And** stderr's footer is `[exit 3: PLUGIN_FETCH_FAILED]`,
- **And** the "what to do" bullets name retry / network checks.

Exercised by `core/internal/plugins/install_test.go` →
`TestInstall_TCPLG010_5xx_surfaces_failure`.

### TC-PLG-011 — `tai plugins list` with no plugins prints `(no plugins installed)`

- **Given** the state file has zero entries (or does not exist),
- **When** the user runs `tai plugins list`,
- **Then** stdout contains the literal `(no plugins installed)`,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/plugins_test.go` →
`TestPluginsList_TCPLG011_empty`.

### TC-PLG-012 — `tai plugins list` renders the installed table

- **Given** the state file records one plugin `triage` version
  `0.5.0` installed at a known timestamp,
- **When** the user runs `tai plugins list`,
- **Then** stdout contains a header row with `name`, `version`, and
  `installed-at`,
- **And** a data row containing `triage` and `0.5.0`,
- **And** the exit code is `0`.

Exercised by `core/internal/cmd/plugins_test.go` →
`TestPluginsList_TCPLG012_renders_table`.

### TC-PLG-013 — Update replaces the binary and re-syncs assets

- **Given** `triage` version `0.4.0` is installed and the state file
  records that source,
- **When** the user runs `tai plugins update triage` and `0.5.0` is
  available,
- **Then** the binary on disk is the `0.5.0` build,
- **And** stale namespaced assets under every configured target are
  removed and re-written from the new bundle,
- **And** the state file's `version` field for `triage` is `0.5.0`.

Exercised by `core/internal/plugins/update_test.go` →
`TestUpdate_TCPLG013_replaces_binary_and_assets`.

### TC-PLG-014 — Remove preserves runtime state and warns

- **Given** `triage` is installed and a runtime-state file exists at
  `<TAI_DATA_DIR>/plugins/triage/state/triage.db`,
- **When** the user runs `tai plugins remove triage`,
- **Then** the plugin binary and `assets/` are deleted,
- **And** every namespaced asset under each configured target is
  removed,
- **And** the entry in `<TAI_DATA_DIR>/state/plugins.json` is gone,
- **And** the `state/triage.db` runtime file is still on disk,
- **And** stderr names the retained path and tells the user how to
  delete it manually.

Exercised at the engine layer by `core/internal/plugins/remove_test.go` →
`TestRemove_TCPLG014_preserves_runtime_state`, with a CLI-boundary
anchor at `core/internal/cmd/plugins_test.go` →
`TestPluginsRemove_TCPLG014_retained_state_warning_at_cli`.

### TC-PLG-015 — `plugins.yml` additive auto-install on `tai sync`

- **Given** the source repo's `plugins.yml` lists `triage` and
  `triage` is not currently installed,
- **When** the user runs `tai sync`,
- **Then** `triage` is installed before the asset-sync phase runs,
- **And** the state file records the new install.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCPLG015_pluginsyml_auto_install`.

### TC-PLG-016 — Removing a `plugins.yml` entry does not uninstall

- **Given** the user has `triage` installed and the source repo's
  `plugins.yml` no longer lists it,
- **When** the user runs `tai sync`,
- **Then** `triage` remains installed,
- **And** no warning or stderr message is produced about the
  missing entry.

Exercised by `core/internal/cmd/sync_test.go` →
`TestSync_TCPLG016_pluginsyml_removal_is_noop`.

<!-- Add new PLG cases here as their proposals land. -->

---

## UB — update banner

The host fires a non-blocking background poll on every invocation
(see TC-SYNC-014..017 for the cadence rule) and, once per calendar
day, prints an aggregated `[tai]`-prefixed banner to stderr naming
every pending update across TAI itself, installed plugins, and the
configured source repo. TAI does not self-update; the banner names
the package-manager command. Spec:
`openspec/specs/update-banner/spec.md`.

### TC-UB-001 — Banner fires on first command of the day when updates are pending

- **Given** `<TAI_DATA_DIR>/state/update-check.json` reports TAI
  `1.3.0` available (current `1.2.0`) and `last-banner-date` is
  yesterday (or absent),
- **When** the user runs any TAI command,
- **Then** stderr contains a banner naming the upgrade,
- **And** the cache's `last-banner-date` is updated to today's
  date in the user's local time zone,
- **And** the foreground command's exit code and stdout are
  unaffected (the banner does not change either).

Exercised at the engine layer by `core/internal/cmd/banner_test.go`
→ `TestBanner_TCUB001_fires_on_first_command`, with a CLI-boundary
wiring anchor at `core/internal/cmd/banner_test.go` →
`TestBanner_TCUB007_fires_at_cli_boundary` (drives `tai --version`
through `runRoot` and asserts the banner reaches `r.stderr` and not
`r.stdout`).

### TC-UB-002 — Banner is suppressed on subsequent commands the same day

- **Given** the cache file's `last-banner-date` equals today's
  date and pending updates remain,
- **When** the user runs another TAI command,
- **Then** stderr contains no `[tai]` banner.

Exercised by `core/internal/cmd/banner_test.go` →
`TestBanner_TCUB002_suppressed_same_day`.

### TC-UB-003 — No banner when nothing is pending

- **Given** the cache file shows `has-updates: false` for every
  layer (TAI, plugins, source-repo),
- **When** the user runs any TAI command,
- **Then** stderr contains no `[tai]` banner regardless of
  `last-banner-date`.

Exercised by `core/internal/cmd/banner_test.go` →
`TestBanner_TCUB003_nothing_pending_no_banner`.

### TC-UB-004 — Banner is stderr-only, prefixed `[tai]`, at most 4 lines

- **Given** the cache file reports updates for TAI, one plugin,
  and the source repo,
- **When** the banner fires,
- **Then** stdout receives no banner text,
- **And** stderr's banner has every line prefixed with `[tai]`,
- **And** the banner is at most 4 lines.

Exercised by `core/internal/cmd/banner_test.go` →
`TestBanner_TCUB004_stderr_only_prefixed_short`.

### TC-UB-005 — Banner names exact update commands per layer

- **Given** the cache file reports TAI, one plugin, and the
  source-repo all have updates,
- **When** the banner fires,
- **Then** stderr names a package-manager command for TAI
  (`brew upgrade tai` or `go install …@latest`),
- **And** stderr names `tai plugins update <name>` for the plugin,
- **And** stderr names `tai sync` for the source-repo.

Exercised by `core/internal/cmd/banner_test.go` →
`TestBanner_TCUB005_names_exact_commands`.

### TC-UB-006 — `tai update` exits with `UNKNOWN_SUBCOMMAND`

- **Given** the user runs `tai update`,
- **When** the command resolves,
- **Then** the exit code is `1`,
- **And** stderr's footer is `[exit 1: UNKNOWN_SUBCOMMAND]`,
- **And** the "what to do" bullets name a package-manager command
  as the resolution (TAI is not self-updating).

Exercised by `core/internal/cmd/banner_test.go` →
`TestBanner_TCUB006_tai_update_is_unknown_subcommand`.

### TC-UB-007 — Banner reaches the user via the CLI entry point

- **Given** the cache file reports a pending TAI update with
  `last-banner-date` set to yesterday,
- **When** the user runs any TAI command via the CLI (e.g.
  `tai --version`),
- **Then** stderr contains the `[tai]` banner,
- **And** stdout does not contain the banner,
- **And** the foreground command's product (the version line) lands
  on stdout unaffected.

This case is the CLI-boundary integration anchor for the banner
emitter; TC-UB-001..005 exercise `sync.EmitBanner` directly with a
captured buffer, but a regression that fails to wire `EmitBanner`
into the host's request path (e.g. wrong stream, wrong dataDir,
call omitted) would be invisible to those unit tests. TC-UB-007
catches the integration regression by driving `runRoot`.

Exercised by `core/internal/cmd/banner_test.go` →
`TestBanner_TCUB007_fires_at_cli_boundary`.

<!-- Add new UB cases here as their proposals land. -->

---

## REL — release cycle & prefix-aware lookups

The release-cycle capability (`openspec/specs/release-cycle/spec.md`,
landed by the `release-cycle` change) pins two contracts that core
must honour at runtime:

1. The release-asset filename for a plugin's tarball MUST equal
   `core/internal/plugins.AssetFilename(name, os, arch)`. The same
   string is produced by GoReleaser (`.goreleaser.<plugin>.yaml`'s
   `archives.name_template`) and consumed by the plugin host's HTTP
   fetcher. Drift breaks `tai plugins install <name>` at runtime —
   these cases catch it at test time instead.

2. The "latest" lookup for any prefixed plugin tag stream
   (`plugins/<name>/v*`) MUST use the list-and-filter algorithm
   defined in the release-cycle spec, NOT GitHub's
   `/repos/{repo}/releases/latest` endpoint. The endpoint returns
   the chronologically newest non-pre-release across the entire
   repo and would cross-contaminate plugin lookups with core
   releases once core ships under bare `v*` tags from the same
   repo.

### TC-REL-001 — `AssetFilename` matches the GoReleaser archive name

- **Given** the release pipeline produces archives via
  `.goreleaser.<plugin>.yaml` with `archives.name_template:
  tai-plugin-<plugin>-{{ .Os }}-{{ .Arch }}` and `format: tar.gz`,
- **When** `core/internal/plugins.AssetFilename(name, os, arch)` is
  called for any plugin name and any `(os, arch)` in the build
  matrix (`{linux, darwin, windows} × {amd64, arm64}`),
- **Then** the returned string is exactly
  `tai-plugin-<name>-<os>-<arch>.tar.gz` for every pair,
- **And** the result equals byte-for-byte the filename present in
  `dist/` after `make release-snapshot`.

Exercised by `core/internal/plugins/fetch_test.go` →
`TestAssetFilename_TCREL001_matches_goreleaser_archive_name`. The
test names every `(os, arch)` pair the spec pins, so any future
edit that loosens the format string (drops the `.tar.gz`,
swaps the dashes for underscores, etc.) flips the test red.

### TC-REL-002 — Prefix-aware lookup ignores releases without the plugin prefix

- **Given** the plugin host queries `<source-repo>/releases` and the
  response contains a mix of tags — some `vX.Y.Z` (core releases,
  unprefixed) and some `plugins/<name>/vA.B.C`,
- **When** the host resolves "latest version for plugin `<name>`",
- **Then** only entries whose `tag_name` starts with
  `plugins/<name>/` are considered,
- **And** the highest semver among those entries is returned,
- **And** unprefixed `vX.Y.Z` entries are silently skipped even when
  they are chronologically newer.

Exercised by `core/internal/plugins/fetch_test.go` →
`TestLatestPrefixed_TCREL002_filters_by_prefix`.

### TC-REL-003 — Prefix-aware lookup drops pre-release tags

- **Given** the only matching plugin tag is
  `plugins/<name>/v0.5.0-rc.1` (the release's `prerelease: true`),
- **When** the host resolves "latest stable version for plugin
  `<name>`",
- **Then** the pre-release entry is dropped,
- **And** the next-highest stable semver is returned,
- **And** if no stable entry exists, the lookup returns the
  "no release" sentinel (callers treat it as "no update
  available", not an error).

Exercised by `core/internal/plugins/fetch_test.go` →
`TestLatestPrefixed_TCREL003_drops_prereleases`.

### TC-REL-004 — Prefix-aware lookup returns the maximum semver, not the chronologically newest

- **Given** the matching plugin tags are `plugins/<name>/v0.5.0`
  (published 2026-06-01) and `plugins/<name>/v0.4.1` (published
  2026-06-15, e.g. a hotfix on a previous line),
- **When** the host resolves "latest version for plugin `<name>`",
- **Then** `v0.5.0` is returned (highest semver),
- **And** publication order is not consulted.

Exercised by `core/internal/plugins/fetch_test.go` →
`TestLatestPrefixed_TCREL004_picks_max_semver`.

### TC-REL-005 — Malformed plugin tags are silently dropped

- **Given** the matching plugin tags are `plugins/<name>/v0.5.0`
  and `plugins/<name>/oops-not-a-version`,
- **When** the host resolves "latest version for plugin `<name>`",
- **Then** the malformed tag is dropped without error or warning,
- **And** `v0.5.0` is returned.

Exercised by `core/internal/plugins/fetch_test.go` →
`TestLatestPrefixed_TCREL005_tolerates_malformed_tags`.

### TC-REL-006 — Banner plugin-row uses the prefix-aware lookup

- **Given** `<TAI_DATA_DIR>/state/plugins.json` records that the
  `triage` plugin is installed at version `v0.4.0`,
- **And** the source repo's `releases` payload contains (newest
  first by `published_at`): `v0.6.1` (core),
  `plugins/triage/v0.5.0`, `v0.6.0` (core), `plugins/triage/v0.4.0`,
- **When** the background update check runs and the banner fires
  the next day,
- **Then** the banner's triage row reads `triage v0.4.0 → v0.5.0`,
- **And** the row never names `v0.6.1` or any other unprefixed
  core release (no cross-contamination).

Exercised at the poll-layer by `core/internal/sync/banner_test.go` →
`TestBanner_TCREL006_plugin_row_uses_prefix_aware_lookup` (asserts
`PollState.Plugins`), and at the CLI boundary by
`core/internal/cmd/banner_test.go` →
`TestBanner_TCREL006_plugin_row_appears_in_stderr` (asserts the
rendered stderr bytes).

### TC-REL-007 — Banner plugin-row suppressed when only pre-release plugin tags exist

- **Given** the installed plugin is `triage v0.4.0`,
- **And** the only newer triage release in the source repo is
  `plugins/triage/v0.5.0-rc.1` (`prerelease: true`),
- **When** the background update check runs,
- **Then** the cache file records no available triage update,
- **And** no `triage` row appears in the banner.

Exercised at the poll-layer by `core/internal/sync/banner_test.go` →
`TestBanner_TCREL007_plugin_row_skips_prereleases` (asserts
`PollState.Plugins` stays empty), and at the CLI boundary by
`core/internal/cmd/banner_test.go` →
`TestBanner_TCREL007_no_plugin_row_when_only_prereleases` (asserts
the rendered stderr is empty).

### TC-REL-008 — Banner core-row keeps using `/releases/latest`

- **Given** the source repo has core releases `v0.6.1`, `v0.6.0`,
  and a pre-release `v0.7.0-rc.1`, with installed core `v0.6.0`,
- **When** the background update check runs,
- **Then** the core lookup hits
  `/repos/{repo}/releases/latest` (NOT the list-and-filter path),
- **And** the cache file records `tai 0.6.0 → 0.6.1` (the endpoint
  already excludes pre-releases for bare-tag streams),
- **And** the banner reflects the same.

Exercised by `core/internal/sync/banner_test.go` →
`TestBanner_TCREL008_core_row_uses_releases_latest`.

### TC-REL-009 — Pseudo-version surfaces as `dev`

- **Given** the binary was built via `go install github.com/dmastrorillo/tai/core/cmd/tai@<commit-or-branch>` or a symlinked-local equivalent that produces a Go pseudo-version in `runtime/debug.ReadBuildInfo().Main.Version` (canonical form: `vX.Y.Z(-<pre>)?[.-]\d{14}-[0-9a-f]{12}`, e.g. `v0.1.2-0.20260609004251-72a773c77386`),
- **When** the user runs `tai --version`,
- **Then** stdout contains `tai version dev`,
- **And** the underlying pseudo-version string is NOT surfaced.

Exercised by `core/internal/version/version_test.go` →
`TestResolveVersion` table cases `pseudo_pre_1_0`, `pseudo_post_1_0_with_zero_prefix`, `pseudo_with_prerelease`.

### TC-REL-010 — Clean release tag passes through

- **Given** the binary was built via `go install github.com/dmastrorillo/tai/core/cmd/tai@v0.6.0` (or any tagged equivalent) such that `Main.Version` is exactly `v0.6.0` or a SemVer pre-release like `v0.6.0-rc.1`,
- **When** the user runs `tai --version`,
- **Then** stdout contains `tai version v0.6.0` (or `v0.6.0-rc.1`),
- **And** the pseudo-version detection does NOT match (pre-release suffixes that lack a 14-digit timestamp + 12-hex sha pass through).

Exercised by `core/internal/version/version_test.go` →
`TestResolveVersion` table cases `dev_buildinfo_real_tag` (already present) and a new `dev_buildinfo_prerelease_clean`.

<!-- Add new REL cases here as their proposals land. -->
