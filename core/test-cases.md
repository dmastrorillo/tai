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
`pivot-to-ai-as-code/specs/config/spec.md`. The data-directory-resolution
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
`openspec/changes/pivot-to-ai-as-code/specs/repo-sync/spec.md`.

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

<!-- Add new SYNC cases here as their proposals land. -->

---

## INIT — repo scaffold

`tai repo init <path>` writes a templated source-repo scaffold,
auto-initialises a git repo, and prints next-step commands. Spec:
`openspec/changes/pivot-to-ai-as-code/specs/repo-init/spec.md`.

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

<!-- Add new INIT cases here as their proposals land. -->

---

## WF — workflows

`tai workflow list/run` exposes YAML workflow files under
`<clone>/workflows/**/*.yml` to AI agents as markdown plans. Spec:
`openspec/changes/pivot-to-ai-as-code/specs/workflows/spec.md`.

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
`openspec/changes/pivot-to-ai-as-code/specs/standards/spec.md`.

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
