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
