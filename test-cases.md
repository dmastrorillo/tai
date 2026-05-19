# test-cases.md — tai behavioural specification

This file is the authoritative, human-readable specification of how tai
behaves. It holds BDD-style Given / When / Then scenarios covering happy
paths, edge cases, and known historical regressions. **It is the contract;
the code is downstream.**

The flow is: **OpenSpec proposal → BDD cases here → tests → production
code → observed CLI behaviour.** A change is "real" only after it appears
as a Given / When / Then below, is exercised by a test that names its
TC-ID, and is implemented behind that test.

See [`CLAUDE.md`](./CLAUDE.md) for the full pipeline and ID-scheme rules.

---

## Categories

Each case has an ID of the form `TC-<CATEGORY>-<NUMBER>`. Categories are
short, stable codes; numbers increment within each category starting at
`001`, zero-padded to 3 digits. **Never renumber existing IDs.**

| Code | Scope |
|------|-------|
| [`CMD`](#cmd--command-wiring--meta-verbs) | Top-level command wiring, help, version, unknown subcommands |
| [`ERR`](#err--error-contract) | Error-message format, error-code taxonomy, exit-code mapping |
| [`CFG`](#cfg--data-directory--config-resolution) | Data-directory resolution, env-var precedence |
| [`REPO`](#repo--repo-context-detection) | `origin` URL parsing, `--repo` flag, scope auto-detect |
| [`STG`](#stg--storage-layer) | SQLite schema, migrations, constraint enforcement |
| [`INST`](#inst--install--uninstall) | `tai install` / `tai uninstall`, hash-ledger reconciliation |
| [`IMP`](#imp--import) | `tai import -`, JSON validation, upsert semantics |
| [`TRG`](#trg--triage-state) | list / show / accept / dismiss / complete / status / forget |

(`/tai:import`, `/tai:triage`, and `/tai:verify` slash commands are
exercised manually — they have no TC-IDs because their conversational
prompt-flow contracts aren't unit-testable from Go. Their bundled
markdowns ARE covered at the install boundary by TC-INST-043,
TC-INST-044, and TC-INST-045.)

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

Exercised by `internal/cmd/root_test.go` → `TestVersion_TCCMD001_prints_version_string`.

### TC-CMD-002 — unknown subcommand / flag exits with `UNKNOWN_SUBCOMMAND`

- **Given** the root command has no subcommand or flag matching the
  user's input,
- **When** the user runs `tai --bogus-flag`, `tai bogus`, or any other
  unrecognised token,
- **Then** stderr contains the standard error template (Error line,
  "What to do:" block with a `tai --help` pointer, and the footer
  `[exit 1: UNKNOWN_SUBCOMMAND]`),
- **And** the exit code is `1`.

Exercised by `internal/cmd/root_error_test.go` →
`TestRoot_TCCMD002_unknown_flag` (flag form) and
`TestRoot_TCCMD002_unknown_positional` (positional form).

### TC-CMD-003 — bundled slash-command frontmatter parses into a populated `Frontmatter`

- **Given** a well-formed bundled command markdown with the six required
  frontmatter keys and a body,
- **When** `cmdframework.Parse` is called,
- **Then** the returned `Frontmatter` has all fields populated,
- **And** the returned body is everything after the closing `---\n`
  with any trailing newline preserved.

Exercised by `internal/cmdframework/cmdframework_test.go` →
`TestParse_TCCMD003_golden_good_frontmatter`.

### TC-CMD-004 — frontmatter missing `content_hash` is rejected

- **Given** a frontmatter that omits the `content_hash` field,
- **When** `cmdframework.Parse` is called,
- **Then** the returned error names `content_hash` as missing.

Exercised by `TestParse_TCCMD004_missing_content_hash`.

### TC-CMD-005 — unknown frontmatter field is rejected

- **Given** a frontmatter that contains a key not in the known six
  (e.g. `priority`),
- **When** `cmdframework.Parse` is called,
- **Then** the returned error names the unknown key.

Exercised by `TestParse_TCCMD005_unknown_field`.

### TC-CMD-006 — `HashBody` is deterministic and trailing-newline sensitive

- **Given** identical input bytes,
- **When** `cmdframework.HashBody` is called twice,
- **Then** the two return values are equal.
- **And** the hash of `hello\n` differs from the hash of `hello`.

Exercised by `TestHashBody_TCCMD006_determinism`.

### TC-CMD-007 — `Ledger("any-verb")` returns an empty slice in the foundation stub

- **Given** no install-time ledger has been populated yet,
- **When** `cmdframework.Ledger(verb)` is called for any verb,
- **Then** the returned slice is empty.

Exercised by `TestLedger_TCCMD007_unknown_verb_returns_empty`.

### TC-CMD-008 — `tai --help` outside a git repository exits 0

- **Given** the current directory is not inside any git repository,
- **When** the user runs `tai --help`,
- **Then** the CLI prints help to stdout (containing `tai`),
- **And** the exit code is `0`,
- **And** the CLI does not invoke the repo resolver (the absence of
  any `REPO_NOT_FOUND` signal in stderr is the user-visible evidence).

Exercised by `internal/cmd/repo_test.go` →
`TestHelp_TCCMD008_outside_git_repo`. The `--version` half of the
"repo-independent meta verbs" requirement is covered by TC-CMD-001 plus
`TestVersion_TCCMD001_outside_git_repo`.

<!-- Add new CMD cases here as their proposals land. -->

---

## ERR — error contract

### TC-ERR-001 — panic during command execution surfaces as `INTERNAL_ERROR`

- **Given** any tai command whose action panics,
- **When** `cliexec.Run` invokes that command,
- **Then** the returned error is a `*errcode.Error` with `Code =
  INTERNAL_ERROR`,
- **And** the message embeds the recovered panic value,
- **And** the mapped exit code is `70`.

Exercised by `internal/cliexec/cliexec_test.go` →
`TestRun_TCERR001_panic_becomes_internal_error`.

### TC-ERR-002 — full error template renders for an `*errcode.Error` with help

- **Given** a `*errcode.Error` constructed with a code, message, and one
  or more help bullets,
- **When** `cliout.WriteError` formats it,
- **Then** the output contains an `Error: <msg>` line, a `What to do:`
  block with one bullet per help entry, and a final `[exit N: CODE]`
  footer on its own line.

Exercised by `internal/cliout/cliout_test.go` →
`TestWriteError_TCERR002_template_with_help`.

### TC-ERR-003 — `INTERNAL_ERROR` with no help bullets omits the "What to do" block

- **Given** a `*errcode.Error` with `Code = INTERNAL_ERROR` and no help
  bullets (the recovered-panic shape),
- **When** `cliout.WriteError` formats it,
- **Then** the output contains the `Error:` line and the `[exit 70:
  INTERNAL_ERROR]` footer,
- **And** the output does NOT contain a `What to do:` block.

Exercised by `internal/cliout/cliout_test.go` →
`TestWriteError_TCERR003_internal_error_omits_help`.

### TC-ERR-004 — footer regex invariant across every error path

- **Given** any error written via `cliout.WriteError`,
- **When** the output is inspected,
- **Then** the last non-empty line matches
  `^\[exit \d+: [A-Z][A-Z0-9_]*\]$`.

Exercised by `internal/cliout/cliout_test.go` →
`TestWriteError_TCERR004_footer_regex_invariant`.

### TC-ERR-005 — panic with an error value preserves the cause chain

- **Given** an action panics with a value that is itself an `error`
  (a sentinel returned from a helper, for instance),
- **When** `cliexec.Run` recovers,
- **Then** the returned error is a `*errcode.Error{Code: INTERNAL_ERROR}`,
- **And** `errors.Is(returned, sentinel)` is true,
- **And** `errors.Unwrap(returned)` returns the sentinel.

Exercised by `internal/cliexec/cliexec_test.go` →
`TestRun_TCERR005_panic_with_error_preserves_cause_chain`.

### TC-ERR-006 — normal action errors pass through unchanged

- **Given** an action returns a non-nil error WITHOUT panicking,
- **When** `cliexec.Run` returns,
- **Then** the returned error is the same value the action produced
  (verified via `errors.Is`).

Exercised by `internal/cliexec/cliexec_test.go` →
`TestRun_TCERR006_passes_through_non_panic_errors`.

### TC-ERR-007 — unstructured errors render as `INTERNAL_ERROR`

- **Given** an arbitrary `error` value that is NOT a `*errcode.Error`,
- **When** `cliout.WriteError` formats it,
- **Then** stderr starts with `Error: <msg>` (the original message),
- **And** the footer is `[exit 70: INTERNAL_ERROR]`.

Exercised by `internal/cliout/cliout_test.go` →
`TestWriteError_TCERR007_unstructured_error_becomes_internal_error`.

### TC-ERR-008 — error messages with embedded newlines collapse to one line

- **Given** an error whose `Error()` returns multiple newline-separated
  lines,
- **When** `cliout.WriteError` formats it,
- **Then** the rendered `Error:` line is a single line containing all
  the original content, separated by spaces (no embedded newlines),
- **And** the footer remains the last line of stderr.

Exercised by `internal/cliout/cliout_test.go` →
`TestWriteError_TCERR008_multiline_message_collapsed_to_single_line`.

---

## CFG — data directory & config resolution

### TC-CFG-001 — default on Linux with no overrides

- **Given** `$XDG_DATA_HOME` and `$TAI_DATA_DIR` are both unset and
  `$HOME` is `/tmp/fake-home`,
- **When** `datadir.Resolve()` runs on Linux/macOS,
- **Then** the resolved path is `/tmp/fake-home/.local/share/tai`.

On Windows, the equivalent: with `LOCALAPPDATA` set to
`C:\Users\test\AppData\Local`, the resolved path is
`C:\Users\test\AppData\Local\tai`.

Exercised by `internal/datadir/datadir_test.go` →
`TestResolve_TCCFG001_default_linux_no_overrides`.

### TC-CFG-002 — `XDG_DATA_HOME` overrides the OS default

- **Given** `$XDG_DATA_HOME` is `/custom/xdg` and `$TAI_DATA_DIR` is
  unset,
- **When** `datadir.Resolve()` runs,
- **Then** the resolved path is `/custom/xdg/tai` (the literal `tai`
  suffix is appended).

Exercised by `internal/datadir/datadir_test.go` →
`TestResolve_TCCFG002_xdg_overrides_default`.

### TC-CFG-003 — `TAI_DATA_DIR` wins over `XDG_DATA_HOME` and is used verbatim

- **Given** `$TAI_DATA_DIR` is `/explicit/tai-data` and `$XDG_DATA_HOME`
  is `/custom/xdg`,
- **When** `datadir.Resolve()` runs,
- **Then** the resolved path is `/explicit/tai-data` exactly — no `tai`
  suffix is appended.

Exercised by `internal/datadir/datadir_test.go` →
`TestResolve_TCCFG003_tai_data_dir_wins`.

### TC-CFG-004 — unwritable data directory surfaces `DATA_DIR_UNWRITABLE`

- **Given** `$TAI_DATA_DIR` points at a path that cannot be created
  (e.g. `/dev/null/cannot-mkdir`),
- **When** `datadir.EnsureWritable()` runs,
- **Then** the returned error is a `*errcode.Error` with
  `Code = DATA_DIR_UNWRITABLE`,
- **And** the error carries at least one remediation help bullet,
- **And** the error message names the failing path.

Exercised by `internal/datadir/datadir_test.go` →
`TestEnsureWritable_TCCFG004_unwritable_dir`.

Integration coverage of the real-filesystem read-only path (chmod 555)
lives in `datadir_integration_test.go` behind the `integration` build tag.

---

## REPO — repo-context detection

### TC-REPO-001 — SSH origin URL normalises to `owner/name`

- **Given** a string of the form `git@github.com:acme/app.git`,
- **When** `repoctx.ParseOriginURL` is called,
- **Then** the returned Identity is `acme/app`.

Exercised by `internal/repoctx/repoctx_test.go` →
`TestParseOriginURL_TCREPO001_ssh`.

### TC-REPO-002 — HTTPS origin URL with `.git` suffix normalises

- **Given** a string of the form `https://github.com/acme/app.git`,
- **When** `repoctx.ParseOriginURL` is called,
- **Then** the returned Identity is `acme/app`.

Exercised by `TestParseOriginURL_TCREPO002_https_with_dot_git`.

### TC-REPO-003 — HTTPS origin URL without `.git` suffix normalises

- **Given** a string of the form `https://github.com/acme/app`,
- **When** `repoctx.ParseOriginURL` is called,
- **Then** the returned Identity is `acme/app`.

Exercised by `TestParseOriginURL_TCREPO003_https_without_dot_git`.

### TC-REPO-004 — outside a git repo, `tai <verb-needing-repo>` fails with `REPO_NOT_FOUND`

- **Given** the working directory is not inside any git repository,
- **And** no `--repo` flag is provided,
- **When** a subcommand that calls `cmd.RequireRepo` is invoked,
- **Then** the CLI exits `2` with the footer
  `[exit 2: REPO_NOT_FOUND]`.

Exercised by `internal/repoctx/repoctx_test.go` →
`TestRead_TCREPO004_not_a_repo` (unit) and
`internal/cmd/repo_test.go` →
`TestRequireRepo_TCREPO004_outside_git_fails` (E2E).

### TC-REPO-005 — git repo with no `origin` remote returns `REPO_NOT_FOUND`

- **Given** the working directory is inside a git repository that has
  no `origin` remote configured,
- **When** `repoctx.Read` runs,
- **Then** the returned error is a `*errcode.Error` with
  `Code = REPO_NOT_FOUND`,
- **And** the help text suggests `git remote add origin`.

Exercised by `TestRead_TCREPO005_no_origin`.

### TC-REPO-006 — `--repo` flag overrides auto-detection

- **Given** the working directory is not inside any git repository,
- **When** the user runs the verb with `--repo acme/app`,
- **Then** the resolver returns the Identity `acme/app` and the
  subcommand action runs normally with exit `0`.

Exercised by `TestResolve_TCREPO006_repo_flag_override` (unit) and
`TestRequireRepo_TCREPO006_flag_override_succeeds` (E2E).

### TC-REPO-007 — malformed `--repo` value yields `REPO_FLAG_INVALID`

- **Given** the user passes `--repo just-a-name` (or another value
  not matching `<owner>/<name>`),
- **When** the resolver parses the flag,
- **Then** the CLI exits `1` with the footer
  `[exit 1: REPO_FLAG_INVALID]`.

Exercised by `TestParseIdentity_TCREPO007_malformed` (unit) and
`TestRequireRepo_TCREPO007_malformed_flag_fails` (E2E).

---

## STG — storage layer

### TC-STG-001 — WAL is the journal mode on every connection

- **Given** a fresh tai.db on a real filesystem,
- **When** `storage.OpenAt` runs,
- **Then** `PRAGMA journal_mode` returns `wal`.

Exercised by `internal/storage/storage_test.go` →
`TestOpen_TCSTG001_WAL_active`.

### TC-STG-002 — `foreign_keys` is on for every connection

- **When** any tai-opened SQLite connection runs `PRAGMA foreign_keys`,
- **Then** the value is `1`.

Exercised by `TestOpen_TCSTG002_foreign_keys_on`.

### TC-STG-003 — open failure surfaces `DB_OPEN_FAILED`

- **Given** a path that cannot be opened (e.g. unwritable directory),
- **When** `storage.OpenAt` is called,
- **Then** the returned error is a `*errcode.Error{Code: DB_OPEN_FAILED}`.

Exercised by `TestOpen_TCSTG003_open_failure`.

### TC-STG-004 — fresh database applies all embedded migrations

- **Given** a freshly-created database,
- **When** the runner finishes,
- **Then** every migration in `migrations/` is recorded in the
  `migrations` table with a monotonically increasing version.

Exercised by `TestMigrations_TCSTG004_fresh_db_applies_all`.

### TC-STG-005 — second open against the same DB is a no-op

- **Given** a database with every migration already applied,
- **When** `OpenAt` is invoked again against the same path,
- **Then** no new rows are added to the `migrations` table.

Exercised by `TestMigrations_TCSTG005_second_open_is_noop`.

### TC-STG-006 — failed migration rolls back, surfaces `DB_MIGRATION_FAILED`

- **Given** a migration whose SQL is syntactically invalid,
- **When** `applyOne` attempts to run it,
- **Then** the returned error is a `*errcode.Error{Code:
  DB_MIGRATION_FAILED}`,
- **And** no row was inserted into the `migrations` table for that
  version (the transaction rolled back).

Exercised by `internal/storage/migrations_internal_test.go` →
`TestMigrations_TCSTG006_failed_migration_rolls_back` (internal-package
test, since the public Open path consumes the embedded migrations
directory which cannot host an invalid file).

### TC-STG-010 — `repos` insert succeeds for a new `owner_name`

Exercised by `TestRepos_TCSTG010_insert`.

### TC-STG-011 — duplicate `repos.owner_name` rejected

A second `INSERT INTO repos` with the same `owner_name` violates the
UNIQUE constraint and surfaces `DB_CONSTRAINT_VIOLATION`. Exercised by
`TestRepos_TCSTG011_unique_owner_name`.

### TC-STG-012 — deleting a repo cascades to PRs and branches

Exercised by `TestRepos_TCSTG012_cascade_to_children`.

### TC-STG-020 — `prs` insert succeeds

Exercised by `TestPRs_TCSTG020_insert`.

### TC-STG-021 — duplicate PR number within a repo rejected

Exercised by `TestPRs_TCSTG021_unique_repo_number`.

### TC-STG-022 — same PR number across two repos is allowed

The `(repo_id, number)` UNIQUE is scoped per repo. Exercised by
`TestPRs_TCSTG022_same_number_different_repos`.

### TC-STG-023 — `head_branch` is NOT NULL

Exercised by `TestPRs_TCSTG023_head_branch_not_null`.

### TC-STG-030 — `branches` insert succeeds

Exercised by `TestBranches_TCSTG030_insert`.

### TC-STG-031 — duplicate branch name within a repo rejected

Exercised by `TestBranches_TCSTG031_unique_repo_name`.

### TC-STG-032 — deleting a repo cascades to branches

Exercised by `TestBranches_TCSTG032_cascade_from_repo`.

### TC-STG-040 — comment with PR parent inserts

Exercised by `TestComments_TCSTG040_pr_parent`.

### TC-STG-041 — comment with branch parent inserts

Exercised by `TestComments_TCSTG041_branch_parent`.

### TC-STG-042 — comment with both parents rejected (XOR CHECK)

Exercised by `TestComments_TCSTG042_both_parents_rejected`.

### TC-STG-043 — comment with no parents rejected (XOR CHECK)

Exercised by `TestComments_TCSTG043_no_parents_rejected`.

### TC-STG-044 — invalid severity value rejected

Exercised by `TestComments_TCSTG044_invalid_severity`.

### TC-STG-045 — invalid status value rejected

Exercised by `TestComments_TCSTG045_invalid_status`.

### TC-STG-046 — invalid category value rejected

Exercised by `TestComments_TCSTG046_invalid_category`.

### TC-STG-047 — missing enrichment column (NOT NULL) rejected

Inserting a comment with `why_fix`, `suggested_fix`, or `consequences`
NULL violates the NOT NULL constraint. Exercised by
`TestComments_TCSTG047_missing_enrichment` (sub-tests for each column).

### TC-STG-048 — deleting a PR cascades to its comments

Exercised by `TestComments_TCSTG048_cascade_from_pr`.

### TC-STG-049 — deleting a branch cascades to its comments

Exercised by `TestComments_TCSTG049_cascade_from_branch`.

### TC-STG-050 — multiple external refs of the same source_kind per comment

When the `external_id` values differ, two refs for the same comment
both persist. Exercised by `TestExternalRefs_TCSTG050_multiple_refs`.

### TC-STG-051 — duplicate `(source_kind, external_id)` rejected

Exercised by `TestExternalRefs_TCSTG051_duplicate_ref_rejected`.

### TC-STG-052 — deleting a comment cascades to its external_refs

Exercised by `TestExternalRefs_TCSTG052_cascade_from_comment`.

### TC-STG-053 — deleting a batch sets member comments' batch_id to NULL

The comment row survives the batch deletion (cascade-set-null, not
cascade-delete). Exercised by
`TestBatches_TCSTG053_delete_sets_batch_id_null`.

### TC-STG-060 — `batches` insert with PR parent

Exercised by `TestBatches_TCSTG060_insert`.

### TC-STG-061 — duplicate `(parent, batch_key)` rejected

Exercised by `TestBatches_TCSTG061_duplicate_key_rejected`.

### TC-STG-062 — batch status `mixed` is a legal enum value

Exercised by `TestBatches_TCSTG062_status_mixed_allowed`.

### TC-STG-063 — batch with no parent rejected (XOR CHECK)

Exercised by `TestBatches_TCSTG063_no_parent_rejected`.

### TC-STG-064 — deleting a PR cascades to its batches

Exercised by `TestBatches_TCSTG064_cascade_from_pr`.

### TC-STG-065 — deleting a branch cascades to its batches

Exercised by `TestBatches_TCSTG065_cascade_from_branch`.

### TC-STG-070 — `ErrConstraint` maps constraint failures to `DB_CONSTRAINT_VIOLATION`

- **Given** a SQL operation that fails with a constraint violation,
- **When** the error passes through `storage.ErrConstraint`,
- **Then** the returned error is a `*errcode.Error{Code:
  DB_CONSTRAINT_VIOLATION}`.

Exercised by `TestErrConstraint_TCSTG070_maps_to_DBConstraintViolation`.

### TC-STG-071..073 — error-code CLI-boundary tests *(deferred)*

The spec's "each error code produces the standard footer" scenarios
([exit 3: DB_OPEN_FAILED] etc.) require a subcommand that opens the
database to be wired in. The first such subcommand lands in
`add-import-command`; the matching footer tests land alongside it.
The internal error-code mapping is locked down by
`internal/errcode`'s taxonomy test (every Code → exit code) and by
`TC-STG-003` / `TC-STG-006` / `TC-STG-070` exercising each storage
code at the storage-layer boundary.

---

## INST — install / uninstall

### TC-INST-001 — ledger files are JSON arrays of `sha256:<hex>` strings, oldest-first

- **Given** any bundled `commands/<verb>.ledger.json` file is read,
- **When** the file is parsed,
- **Then** every entry matches `^sha256:[0-9a-f]{64}$`,
- **And** the array order is oldest-first (the current build's body hash
  is the last element).

Exercised by `internal/cmdframework/ledger_test.go` →
`TestLedger_TCINST001_well_formed_entries`.

### TC-INST-002 — `Ledger(unknown)` returns an empty slice

- **Given** a verb name with no matching `commands/<verb>.ledger.json`
  in the embedded bundle,
- **When** `cmdframework.Ledger(verb)` is called,
- **Then** the returned slice is empty.

Exercised by `TestLedger_TCINST002_missing_verb_empty`.

### TC-INST-003 — the current build's body hash is the last entry in each ledger

- **Given** every bundled `commands/<verb>.md` shipped in this build,
- **When** the body hash is computed via `cmdframework.HashBody`,
- **Then** the resulting hash equals the last element of
  `commands/<verb>.ledger.json`.

Exercised by `TestBundle_TCINST003_current_hash_is_last_entry`.

### TC-INST-010 — classifier returns `missing` when the target file does not exist

- **Given** the target path does not exist on disk,
- **When** `installer.Classify(targetPath, ledger)` runs,
- **Then** the returned classification is `missing`.

Exercised by `internal/installer/classifier_test.go` →
`TestClassify_TCINST010_missing`.

### TC-INST-011 — classifier returns `up-to-date` when the body hash matches the current build's hash

- **Given** the target file on disk has the same body bytes as the
  bundled command,
- **When** `installer.Classify(targetPath, ledger)` runs,
- **Then** the returned classification is `up-to-date`.

Exercised by `TestClassify_TCINST011_up_to_date`.

### TC-INST-012 — classifier returns `stale-but-untouched` when the body hash is in the ledger but not current

- **Given** the target file on disk has a body whose hash appears in
  the ledger but is not the last (current) entry,
- **When** `installer.Classify(targetPath, ledger)` runs,
- **Then** the returned classification is `stale-but-untouched`.

Exercised by `TestClassify_TCINST012_stale_but_untouched`.

### TC-INST-013 — classifier returns `user-modified` when the body hash is absent from the ledger

- **Given** the target file's body hash does not appear in the ledger,
- **When** `installer.Classify(targetPath, ledger)` runs,
- **Then** the returned classification is `user-modified`.

Exercised by `TestClassify_TCINST013_user_modified`.

### TC-INST-014 — classifier treats an unparseable target file as `user-modified`

- **Given** the target file exists but cannot be parsed as
  frontmatter+body (no leading `---`, garbage content, …),
- **When** `installer.Classify(targetPath, ledger)` runs,
- **Then** the returned classification is `user-modified` (the most
  conservative label — we never assume ownership of something we
  cannot recognise).

Exercised by `TestClassify_TCINST014_unparseable_file_is_user_modified`.

### TC-INST-020 — `tai install` writes every bundled command on a fresh target

- **Given** the target directory does not yet contain any tai command,
- **When** the user runs `tai install`,
- **Then** every bundled command's markdown is written under the target
  directory,
- **And** stdout contains an `Installed: N command(s)` summary line,
- **And** the exit code is `0`.

Exercised by `internal/cmd/install_test.go` →
`TestInstall_TCINST020_fresh_install`.

### TC-INST-021 — `tai install` re-run on an up-to-date target is a no-op

- **Given** every bundled command has been installed already,
- **When** the user runs `tai install` again,
- **Then** the summary reports `Installed: 0` and the rest under
  `Skipped (up to date)`,
- **And** no file modification times change.

Exercised by `TestInstall_TCINST021_idempotent_rerun`.

### TC-INST-022 — `tai install` overwrites `stale-but-untouched` silently

- **Given** a target file's body hash matches an older ledger entry
  (not the current one),
- **When** `tai install` runs without `--force`,
- **Then** the file is overwritten without a prompt,
- **And** the summary reports it under `Updated`.

Exercised by `TestInstall_TCINST022_stale_overwritten`.

### TC-INST-023 — `tai install --force` overwrites `user-modified` without prompting

- **Given** a target file's body hash is absent from the ledger,
- **When** the user runs `tai install --force`,
- **Then** the file is overwritten with no prompt,
- **And** the summary reports it under `Updated`.

Exercised by `TestInstall_TCINST023_force_overwrites_modified`.

### TC-INST-024 — `TAI_ACCEPT_COMMAND_UPDATES=1` overwrites `user-modified` without prompting

- **Given** a target file's body hash is absent from the ledger,
- **And** `TAI_ACCEPT_COMMAND_UPDATES=1` is set in the environment,
- **When** the user runs `tai install`,
- **Then** the file is overwritten with no prompt,
- **And** the summary reports it under `Updated`.

Exercised by `TestInstall_TCINST024_env_overwrites_modified`.

### TC-INST-025 — `TAI_ACCEPT_COMMAND_UPDATES` set to a non-truthy value is ignored

- **Given** a target file's body hash is absent from the ledger,
- **And** `TAI_ACCEPT_COMMAND_UPDATES=0` (or `false`, `no`, `off`, or an
  unrecognised string) is set in the environment,
- **And** stdin is not a TTY,
- **When** the user runs `tai install`,
- **Then** the file is left unchanged,
- **And** the summary reports it under `Prompted-skipped`,
- **And** the exit code is `0`.

Exercised by `TestInstall_TCINST025_env_non_truthy_ignored`.

### TC-INST-026 — `--commands-dir` overrides the default target

- **Given** the user provides `--commands-dir /tmp/cmds` to a writable
  directory,
- **When** `tai install --commands-dir /tmp/cmds` runs,
- **Then** the bundled commands are written under `/tmp/cmds/`.

Exercised by `TestInstall_TCINST026_commands_dir_override`.

### TC-INST-027 — unwritable target surfaces `INSTALL_TARGET_UNWRITABLE`

- **Given** the target directory is on a read-only filesystem or cannot
  be created,
- **When** `tai install` runs,
- **Then** the CLI exits with code `3`,
- **And** the stderr footer is `[exit 3: INSTALL_TARGET_UNWRITABLE]`.

Exercised by `TestInstall_TCINST027_unwritable_target`.

### TC-INST-028 — malformed `--commands-dir` surfaces `INSTALL_INVALID_TARGET`

- **Given** the user runs `tai install --commands-dir ""`,
- **When** the CLI parses the flag,
- **Then** the CLI exits with code `1`,
- **And** the stderr footer is `[exit 1: INSTALL_INVALID_TARGET]`.

Exercised by `TestInstall_TCINST028_invalid_commands_dir`.

### TC-INST-029 — non-interactive stdin without override skips `user-modified` files

- **Given** a target file's body hash is absent from the ledger,
- **And** stdin is not a TTY,
- **And** neither `--force` nor `TAI_ACCEPT_COMMAND_UPDATES=1` is set,
- **When** `tai install` runs,
- **Then** the file is left in place,
- **And** the summary reports it under `Prompted-skipped`,
- **And** the exit code is `0`.

Exercised by `TestInstall_TCINST029_non_interactive_skip`.

### TC-INST-030 — `tai uninstall` removes every recognised tai command

- **Given** the target directory contains only files whose body hash
  is in the corresponding verb's ledger,
- **When** `tai uninstall` runs,
- **Then** every file is removed,
- **And** the now-empty directory is removed,
- **And** the summary reports each verb under `Removed`,
- **And** the exit code is `0`.

Exercised by `internal/cmd/uninstall_test.go` →
`TestUninstall_TCINST030_clean_uninstall`.

### TC-INST-031 — `tai uninstall` leaves user-modified files in place

- **Given** the target directory contains `<verb>.md` whose body hash
  is not in the ledger,
- **When** `tai uninstall` runs without `--force` and without
  `TAI_ACCEPT_COMMAND_UPDATES=1`,
- **Then** the file is preserved,
- **And** the summary reports it under `Prompted-skipped`.

Exercised by `TestUninstall_TCINST031_leaves_modified_in_place`.

### TC-INST-032 — `tai uninstall --force` removes user-modified files

- **Given** the target directory contains `<verb>.md` whose body hash
  is not in the ledger,
- **When** `tai uninstall --force` runs,
- **Then** the file is removed,
- **And** the summary reports it under `Removed`.

Exercised by `TestUninstall_TCINST032_force_removes_modified`.

### TC-INST-033 — `TAI_ACCEPT_COMMAND_UPDATES=1` removes user-modified files on uninstall

- **Given** a target file whose body hash is not in the ledger,
- **And** `TAI_ACCEPT_COMMAND_UPDATES=1` is set in the environment,
- **When** `tai uninstall` runs without `--force`,
- **Then** the file is removed.

Exercised by `TestUninstall_TCINST033_env_removes_modified`.

### TC-INST-034 — `tai uninstall` preserves files unrelated to known verbs

- **Given** the target directory contains a file whose filename does
  not match any bundled verb,
- **When** `tai uninstall` runs,
- **Then** the file is preserved untouched.

Exercised by `TestUninstall_TCINST034_unrelated_preserved`.

### TC-INST-035 — uninstall preserves the directory when non-empty after processing

- **Given** the target directory still contains unrelated files after
  the known verb files have been removed,
- **When** `tai uninstall` runs,
- **Then** the directory itself is preserved.

Exercised by `TestUninstall_TCINST035_dir_preserved_when_non_empty`.

### TC-INST-036 — uninstall removes the empty directory

- **Given** the target directory contains only bundled tai command
  files (all in their ledgers),
- **When** `tai uninstall` runs,
- **Then** the now-empty directory is removed.

Exercised by `TestUninstall_TCINST036_empty_dir_removed`.

### TC-INST-040 — `tai install` runs outside a git repository

- **Given** the working directory is not inside any git repository,
- **When** `tai install` runs,
- **Then** the command succeeds with no `REPO_NOT_FOUND` footer,
- **And** the exit code is `0`.

Exercised by `TestInstall_TCINST040_outside_git_repo`.

### TC-INST-041 — `tai install` does not touch the data directory

- **Given** `TAI_DATA_DIR=/some/foreign/dir` is set,
- **When** `tai install` runs,
- **Then** no SQLite database is created under `TAI_DATA_DIR`,
- **And** the data directory tree is unchanged.

Exercised by `TestInstall_TCINST041_does_not_touch_data_dir`.

### TC-INST-042 — `tai uninstall` runs outside a git repository

- **Given** the working directory is not inside any git repository,
- **When** `tai uninstall` runs,
- **Then** the command succeeds with no `REPO_NOT_FOUND` footer,
- **And** the exit code is `0`.

Exercised by `internal/cmd/uninstall_test.go` →
`TestUninstall_TCINST042_outside_git_repo`.

### TC-INST-043 — bundled `import.md` flows through `tai install` and classifies up-to-date

- **Given** the production bundle (real `internal/cmdframework/commands/import.md` + its ledger),
- **And** an empty target directory (`installer.Classify(<dir>/import.md, ledger)` returns `missing`),
- **When** `tai install --commands-dir <fresh-dir>` runs,
- **Then** the run exits `0` and the summary mentions `import`,
- **And** immediately afterwards `installer.Classify(<dir>/import.md, ledger)` returns `up-to-date`.

This is the production-bundle counterpart to the fake-bundle install
tests (TC-INST-020..029) — it pins the round-trip from the embedded
`commands/import.md` body through the install writer back to the
classifier. Exercised by `internal/cmd/install_import_smoke_test.go` →
`TestInstall_TCINST043_import_command_bundled` (which delegates the
`missing → install → up-to-date` chain to `runBundledInstallSmoke`,
the helper shared with TC-INST-044).

### TC-INST-044 — bundled `triage.md` flows through `tai install` and tracks classifier transitions

- **Given** the production bundle (real `internal/cmdframework/commands/triage.md` + its ledger),
- **And** an empty target directory (`installer.Classify(<dir>/triage.md, ledger)` returns `missing`),
- **When** `tai install --commands-dir <fresh-dir>` runs,
- **Then** the run exits `0` and the summary mentions `triage`,
- **And** `installer.Classify(<dir>/triage.md, ledger)` is `up-to-date`,
- **And** appending one byte to the installed file flips the classifier to `user-modified`.

The triage counterpart to TC-INST-043 — pins the same `missing →
up-to-date` round-trip for the second bundled verb via the shared
`runBundledInstallSmoke` helper, and additionally exercises the
`up-to-date → user-modified` transition that drives the on-rerun
prompt. Exercised by `internal/cmd/install_triage_smoke_test.go` →
`TestInstall_TCINST044_triage_command_bundled`.

### TC-INST-045 — bundled `verify.md` flows through `tai install` and classifies up-to-date

- **Given** the production bundle (real `internal/cmdframework/commands/verify.md` + its ledger),
- **And** an empty target directory (`installer.Classify(<dir>/verify.md, ledger)` returns `missing`),
- **When** `tai install --commands-dir <fresh-dir>` runs,
- **Then** the run exits `0` and the summary mentions `verify`,
- **And** immediately afterwards `installer.Classify(<dir>/verify.md, ledger)` returns `up-to-date`.

The third bundled-verb install round-trip; uses the same shared
`runBundledInstallSmoke` helper as TC-INST-043 and TC-INST-044. The
post-install `user-modified` flip is already pinned by TC-INST-044
and does not need a third copy here. Exercised by
`internal/cmd/install_verify_smoke_test.go` →
`TestInstall_TCINST045_verify_command_bundled`.

### TC-INST-046 — one `tai install` run lands every bundled verb as `up-to-date`

- **Given** the production bundle (every verb returned by `cmdframework.Verbs()` with its ledger),
- **And** an empty target directory,
- **When** `tai install --commands-dir <fresh-dir>` runs ONCE,
- **Then** for EVERY verb `cmdframework.Verbs()` reports, `installer.Classify(<dir>/<verb>.md, ledger)` returns `up-to-date` afterwards.

The cross-verb companion to TC-INST-043/044/045: those three tests
each run an install but only classify their own verb. A regression
that wrote some verbs correctly and broke others would slip past the
per-verb tests; this test catches that by classifying every bundled
verb after a single install run. Exercised by
`internal/cmd/install_all_bundled_test.go` →
`TestInstall_TCINST046_all_bundled_verbs_up_to_date`.

<!-- Coverage notes:

- TC-INST-025 verifies the env-var wiring at the CLI boundary using a
  single representative non-truthy value (`0`). The full truthy/falsy
  table is exercised exhaustively by the unit test
  `TestIsTruthyEnv` in `internal/installer/env_test.go`. The split
  exists because the CLI-boundary surface is the wiring, not the
  parsing.

- The interactive `[y/N]` prompt path (`IsTTY=true`) is exercised at
  the engine boundary by `TestInstall_interactive_prompt_accept` and
  `TestInstall_interactive_prompt_decline` in
  `internal/installer/run_test.go`. There is no E2E variant via
  `cmdtest.Run` because the harness wires a `strings.Reader` for stdin
  (which is non-TTY); a real terminal cannot be synthesised inside
  `go test` without an OS-level pty. The wiring from `c.Reader` →
  `opts.IsTTY` is covered indirectly by TC-INST-029 (non-TTY skip).
-->


---

## IMP — import

### TC-IMP-001 — well-formed PR payload accepted

- **Given** a JSON payload with `target.kind=pr`, a complete `pr` body,
  one batch, and one well-formed comment piped on stdin,
- **When** `tai import -` runs,
- **Then** the CLI succeeds with exit code `0`,
- **And** the comment is persisted with `status='pending'`.

Exercised by `internal/import/payload/payload_test.go` →
`TestDecodeValidate_TCIMP001_well_formed_PR` (payload validation) and
`internal/cmd/import_test.go` → `TestImport_TCIMP020_stdin_payload_persists`
(end-to-end persistence).

### TC-IMP-002 — well-formed branch payload accepted

- **Given** a payload with `target.kind=branch` and `branch.name`,
- **When** `tai import -` runs,
- **Then** the CLI succeeds with exit `0`,
- **And** the corresponding `branches` row exists in the database.

Exercised at the payload-validation layer by
`TestDecodeValidate_TCIMP002_well_formed_branch` and at the CLI
boundary (including the DB-row check) by
`internal/cmd/import_test.go` → `TestImport_TCIMP081_branch_header_format`.

### TC-IMP-003 — missing `why_fix` rejected

- **Given** a comment whose `why_fix` field is absent or empty,
- **When** the payload is validated,
- **Then** the validator reports `comments[N].why_fix`.

Exercised by `TestValidate_TCIMP003_missing_why_fix_rejected`.

### TC-IMP-004 — invalid severity rejected

- **Given** a comment whose `severity` is not in the enum (e.g. `urgent`),
- **When** the payload is validated,
- **Then** the validator reports `comments[N].severity`.

Exercised by `TestValidate_TCIMP004_invalid_severity_rejected`.

### TC-IMP-005 — target with both pr and branch rejected

- **Given** a `target` carrying both `pr` and `branch` bodies,
- **When** the payload is validated,
- **Then** the validator reports the violation under `target.branch`
  (the "must be absent" path).

Exercised by `TestValidate_TCIMP005_both_pr_and_branch`.

### TC-IMP-006 — target.kind=pr without pr body rejected

- **Given** a payload with `target.kind=pr` but no `target.pr` object,
- **When** the payload is validated,
- **Then** the validator reports `target.pr`.

Exercised by `TestValidate_TCIMP006_kind_pr_without_pr_body`.

### TC-IMP-007 — missing pr.number rejected

- **Given** a payload with `target.pr.number = 0`,
- **When** the payload is validated,
- **Then** the validator reports `target.pr.number`.

Exercised by `TestValidate_TCIMP007_missing_pr_number`.

### TC-IMP-008 — empty external_refs rejected

- **Given** a comment whose `external_refs` array is empty,
- **When** the payload is validated,
- **Then** the validator reports `comments[N].external_refs`.

Exercised by `TestValidate_TCIMP008_empty_external_refs`.

### TC-IMP-009 — unknown field rejected at decode time

- **Given** a JSON payload containing a key not in the schema (e.g.
  `"priority": "p0"` on a comment),
- **When** the payload is decoded,
- **Then** the decoder returns an error naming the unknown key (the CLI
  maps this to `IMPORT_INVALID_JSON`).

Exercised by `TestDecode_TCIMP009_unknown_field`.

### TC-IMP-010 — multiple violations reported in one message

- **Given** a payload with two distinct violations (e.g. invalid
  severity and empty `why_fix`),
- **When** the payload is validated,
- **Then** both violations appear in the returned slice,
- **And** when piped to `tai import -`, every violation path appears
  in stderr (rendered as separate "What to do:" bullets per the
  foundation contract).

Exercised at the engine layer by `TestValidate_TCIMP010_multiple_errors`
and at the CLI boundary by `TestImport_TCIMP010_multi_violation_body`.

### TC-IMP-011 — `batch_key` references unknown batch rejected

- **Given** a comment whose `batch_key` does not match any
  `batches[].batch_key` in the payload,
- **When** the payload is validated,
- **Then** the validator reports `comments[N].batch_key`.

Exercised by `TestValidate_TCIMP011_batch_key_unknown_rejected`.

### TC-IMP-012 — `FormatErrors` renders violations alphabetically by path

- **Given** a `[]ValidationError` containing entries with paths in any
  order,
- **When** `payload.FormatErrors` renders the slice,
- **Then** the output lists violations in lexicographic order by
  `Path`, so identical payloads always produce identical error bodies.

Exercised by `TestFormatErrors_TCIMP012_renders_alphabetical`.

### TC-IMP-013 — engine decoder rejects malformed JSON

- **Given** stdin bytes that are not valid JSON,
- **When** `payload.DecodeBytes` runs,
- **Then** the decoder returns a non-nil error (the CLI maps this to
  `IMPORT_INVALID_JSON`; see TC-IMP-030 for the CLI-boundary half).

Exercised by `TestDecode_TCIMP013_malformed_JSON`.

### TC-IMP-020 — `tai import -` reads stdin and persists

- **Given** a valid payload piped to stdin,
- **When** `tai import -` runs,
- **Then** the CLI exits `0`, prints the success summary on stdout, and
  the row appears in the SQLite database.

Exercised by `TestImport_TCIMP020_stdin_payload_persists`.

### TC-IMP-021 — `tai import` with no positional fails

- **Given** the user invokes `tai import` with no positional argument,
- **When** the CLI runs,
- **Then** the CLI exits `1` with `UNKNOWN_SUBCOMMAND` and the stderr
  footer is `[exit 1: UNKNOWN_SUBCOMMAND]`.

Exercised by `TestImport_TCIMP021_missing_positional_fails`.

### TC-IMP-022 — `tai import 142` (wrong positional) fails

- **Given** the user invokes `tai import 142`,
- **When** the CLI runs,
- **Then** the CLI exits `1` with `UNKNOWN_SUBCOMMAND`.

Exercised by `TestImport_TCIMP022_wrong_positional_fails`.

### TC-IMP-023 — `tai import` rejects `--repo`

- **Given** the user invokes `tai --repo acme/other import -`,
- **When** the CLI runs,
- **Then** the CLI exits `1` with `UNKNOWN_SUBCOMMAND` (the repo flag is
  not accepted by import; the JSON's `repo` field is authoritative).

Exercised by `TestImport_TCIMP023_rejects_repo_flag`.

### TC-IMP-024 — `tai import` succeeds outside a git repository

- **Given** the working directory is not inside any git repository,
- **And** a valid payload is piped to stdin,
- **When** `tai import -` runs,
- **Then** the CLI exits `0`,
- **And** stderr does NOT mention `REPO_NOT_FOUND` (repo identity is
  read from the payload, no git resolution happens).

Exercised by `TestImport_TCIMP024_outside_git_repo`.

### TC-IMP-030 — `IMPORT_INVALID_JSON` standard footer

- **Given** stdin contains malformed JSON,
- **When** `tai import -` runs,
- **Then** the CLI exits `1` with `IMPORT_INVALID_JSON` and the stderr
  footer is `[exit 1: IMPORT_INVALID_JSON]`.

Exercised by `TestImport_TCIMP030_invalid_json_footer`.

### TC-IMP-031 — `IMPORT_SCHEMA_INVALID` standard footer

- **Given** a payload that decodes but fails one or more schema rules,
- **When** `tai import -` runs,
- **Then** the CLI exits `3` with `IMPORT_SCHEMA_INVALID` and the error
  body lists every violation.

Exercised by `TestImport_TCIMP031_schema_invalid_footer`.

### TC-IMP-032 — `IMPORT_AMBIGUOUS_REFS` standard footer

- **Given** a payload whose `external_refs` resolve to two distinct
  existing `comments.id` rows,
- **When** `tai import -` runs,
- **Then** the CLI exits `3` with `IMPORT_AMBIGUOUS_REFS`,
- **And** the rendered stderr contains both conflicting comment IDs.

Exercised by `TestImport_TCIMP032_ambiguous_refs_footer` (which checks
the footer and asserts both IDs appear in stderr).

### TC-IMP-040 — first import creates repo and PR rows

- **Given** an empty database,
- **When** a valid PR payload is imported,
- **Then** rows are created in `repos` (with the payload's `owner_name`)
  and `prs` (with the payload's `number`, `title`, `url`, `head_branch`).

Exercised by `internal/import/import_test.go` →
`TestImport_TCIMP040_creates_repo_and_pr`.

### TC-IMP-041 — re-import preserves `repos.created_at`

- **Given** a `repos` row created at time `T1`,
- **When** a re-import for the same repo runs at time `T2 > T1`,
- **Then** `repos.created_at` remains `T1`.

Exercised by `TestImport_TCIMP041_reimport_preserves_repo_created_at`.

### TC-IMP-042 — PR title not overwritten on re-import

- **Given** an existing `prs` row with `title='old'`,
- **When** the payload's PR has `title='new'`,
- **Then** the row's `title` remains `'old'`.

Exercised by `TestImport_TCIMP042_pr_title_not_overwritten`.

### TC-IMP-043 — branch row created on first branch-scope import

- **Given** an empty database,
- **When** a branch-scope payload is imported,
- **Then** a `branches` row is created.

Exercised by `TestImport_TCIMP043_branch_row_created`.

### TC-IMP-050 — new batch inserted

- **Given** a payload whose `batches[]` contains a `batch_key` not
  present in the database,
- **When** the import runs,
- **Then** a new `batches` row is inserted with `status='pending'`.

Exercised by `TestImport_TCIMP050_new_batch_inserted`.

### TC-IMP-051 — existing batch title updated; status preserved

- **Given** an existing batch `(pr_id, batch_key)` with `title='old'`
  and `status='accepted'`,
- **When** the payload contains the same batch key with `title='new'`,
- **Then** the row's `title` becomes `'new'`,
- **And** the row's `status` remains `'accepted'`.

Exercised by `TestImport_TCIMP051_existing_batch_title_updated`.

### TC-IMP-060 — new comment inserted

- **Given** a comment whose `external_refs` do not match any existing
  row,
- **When** the import runs,
- **Then** a new `comments` row is inserted with `status='pending'`,
- **And** every `external_refs` entry is inserted.

Exercised by `TestImport_TCIMP060_new_comment_inserted`.

### TC-IMP-061 — pending comment refreshed on re-import

- **Given** a comment whose `external_refs` match an existing row with
  `status='pending'`,
- **When** the payload contains a different `description`,
- **Then** the row's `description` is updated to match the payload,
- **And** the row's `status` remains `pending`.

Exercised by `TestImport_TCIMP061_pending_refresh`.

### TC-IMP-062 — accepted comment is frozen

- **Given** a comment whose `external_refs` match an existing row with
  `status='accepted'`,
- **When** the payload contains a different `description`,
- **Then** the row's `description` is NOT updated,
- **And** the "Frozen" counter increments.

Exercised by `TestImport_TCIMP062_accepted_frozen`.

### TC-IMP-063 — dismissed comment is frozen

Same as TC-IMP-062 with `status='dismissed'`. Exercised by
`TestImport_TCIMP063_dismissed_frozen`.

### TC-IMP-064 — completed comment is frozen

Same as TC-IMP-062 with `status='completed'`. Exercised by
`TestImport_TCIMP064_completed_frozen`.

### TC-IMP-065 — ambiguous refs rejected

- **Given** two distinct existing `comments.id` rows, each with a
  different `external_ref`,
- **When** a payload arrives whose comment carries both of those refs,
- **Then** the importer returns `*AmbiguousRefsError` naming the
  conflicting IDs; the CLI surfaces `IMPORT_AMBIGUOUS_REFS`.

Exercised by `TestImport_TCIMP065_ambiguous_refs_rejected` (engine) and
`TestImport_TCIMP032_ambiguous_refs_footer` (CLI surface).

### TC-IMP-066 — refs added increments the counter

- **Given** an existing pending comment with one ref,
- **When** a re-import contains the same comment plus an additional
  `external_ref`,
- **Then** the new ref is attached and the "Refs added" counter
  reflects the addition.

Exercised by `TestImport_TCIMP066_refs_added`.

### TC-IMP-070 — transaction rolls back on failure

- **Given** a multi-comment payload where the third comment violates a
  storage constraint,
- **When** the import runs,
- **Then** the importer returns a `*errcode.Error{Code:
  DB_CONSTRAINT_VIOLATION}` (which the CLI surfaces as exit `3` with
  the standard `[exit 3: DB_CONSTRAINT_VIOLATION]` footer),
- **And** no rows from this payload persist.

Exercised at the engine layer by `TestImport_TCIMP070_transaction_rolls_back`.
The CLI-boundary wiring from a returned `*errcode.Error` through
`cliout.WriteError` to the footer is exercised generically by
TC-IMP-030..032 (each running through the same return-path) — the
payload-validator guards make it infeasible to drive a constraint
violation through the CLI from a schema-clean payload, so the engine
test is the authoritative coverage for this scenario.

### TC-IMP-080 — PR header format

- **Given** a payload with `kind=pr, number=142, repo=acme/app`,
- **When** the import succeeds,
- **Then** the summary's first line is `Imported acme/app PR #142 (…)`.

Exercised by `TestImport_TCIMP080_pr_header_format`.

### TC-IMP-081 — branch header format

- **Given** a payload with `kind=branch, name=feat/x, repo=acme/app`,
- **When** the import succeeds,
- **Then** the summary's first line is `Imported acme/app branch feat/x (…)`.

Exercised by `TestImport_TCIMP081_branch_header_format`.

### TC-IMP-082 — empty payload summary

- **Given** a valid payload with `comments=[]` and `batches=[]`,
- **When** the import runs,
- **Then** the summary's header reads `Imported … (0 comments, 0 batches)`,
- **And** the per-counter lines are suppressed.

Exercised by `TestImport_TCIMP082_empty_payload`.

### TC-IMP-083 — empty payload upserts repo and target rows (engine)

- **Given** a valid payload with `comments=[]` and `batches=[]`,
- **When** `importer.Import` runs against an empty database,
- **Then** rows are created in `repos` and the appropriate target
  table (`prs` or `branches`),
- **And** the returned `Summary` reports zero comments and zero batches.

Exercised by `internal/import/import_test.go` →
`TestImport_TCIMP083_empty_payload_succeeds`. (The CLI-boundary half
is TC-IMP-082.)

---

## TRG — triage state

### TC-TRG-001 — `--pr` flag scopes the verb

- **Given** a PR has been imported,
- **When** `tai list --pr <N>` runs,
- **Then** the resolved scope is that PR and the header reads
  `Scope: PR #<N>`.

Exercised by `internal/cmd/scope_test.go` →
`TestScope_TCTRG001_pr_flag`.

### TC-TRG-002 — `--branch` flag scopes the verb

- **Given** a branch has been imported,
- **When** `tai list --branch <name>` runs,
- **Then** the resolved scope is that branch and the header reads
  `Scope: branch <name>`.

Exercised by `TestScope_TCTRG002_branch_flag`.

### TC-TRG-003 — auto-detect resolves to PR when `head_branch` matches

- **Given** a PR exists with `head_branch = feat/x`,
- **And** the current git branch is `feat/x`,
- **And** no `branches` row exists with `name = feat/x`,
- **When** a triage verb runs without `--pr` / `--branch`,
- **Then** the resolved scope is that PR.

Exercised by `internal/cmd/scope_test.go` →
`TestScope_TCTRG003_auto_detect_pr`.

### TC-TRG-004 — auto-detect resolves to branch when name matches

- **Given** a `branches` row exists with `name = feat/y`,
- **And** the current git branch is `feat/y`,
- **And** no `prs.head_branch` matches `feat/y`,
- **When** a triage verb runs without `--pr` / `--branch`,
- **Then** the resolved scope is that branch.

Exercised by `TestScope_TCTRG004_auto_detect_branch`.

### TC-TRG-005 — auto-detect with both matches fails `TRIAGE_AMBIGUOUS_SCOPE`

- **Given** both a `prs.head_branch = feat/z` and a `branches.name = feat/z` row exist,
- **And** the current git branch is `feat/z`,
- **When** a triage verb runs without `--pr` / `--branch`,
- **Then** the CLI exits `2` with `TRIAGE_AMBIGUOUS_SCOPE`.

Exercised by `TestScope_TCTRG005_auto_detect_ambiguous`.

### TC-TRG-006 — auto-detect fails with `TRIAGE_NO_SCOPE`

- **Given** the working directory is not in a git repo and the
  current branch maps to neither a `prs.head_branch` nor a
  `branches.name` row,
- **When** `tai list` runs with `--repo` but no `--pr`/`--branch`,
- **Then** the CLI exits `2` with `TRIAGE_NO_SCOPE`.

Exercised by `TestScope_TCTRG006_no_scope`.

### TC-TRG-007 — `--pr` + `--branch` mutex

- **Given** a triage verb is invoked with both `--pr` and `--branch`,
- **When** the CLI parses flags,
- **Then** it exits `1` with `TRIAGE_INVALID_FLAGS`.

Exercised by `TestScope_TCTRG007_mutex`.

### TC-TRG-010 — positions start at 1 within a target

- **Given** a target has one or more comments,
- **When** `tai list` runs,
- **Then** the first row's ID column reads `1`.

Exercised by `TestPosition_TCTRG010_starts_at_1`.

### TC-TRG-012 — positions renumber after delete

- **Given** a target with three comments at positions 1, 2, 3,
- **When** `tai forget --comment 2 --yes` runs,
- **Then** the remaining comments are displayed as 1 and 2.

Exercised by `TestPosition_TCTRG012_shift_after_delete`.

### TC-TRG-020 — `tai list` renders a row per comment

- **Given** a scope with one or more comments,
- **When** `tai list` runs,
- **Then** stdout contains the `Repo:` / `Scope:` header and a row
  for each comment.

Exercised by `internal/cmd/list_test.go` → `TestList_TCTRG020_with_comments`.

### TC-TRG-021 — `tai list` empty scope prints `(no comments)`

- **Given** a scope with zero comments,
- **When** `tai list` runs,
- **Then** stdout contains the header followed by `(no comments)`.

Exercised by `TestList_TCTRG021_empty_scope`.

### TC-TRG-022 — severity is abbreviated

- **Given** comments span the severity enum,
- **When** `tai list` runs,
- **Then** stdout shows `crit`, `maj`, `min`, `nit` (not the full
  words) in the SEV column.

Exercised by `TestList_TCTRG022_severity_abbreviated`.

### TC-TRG-023 — multiple `--status` values combine via OR

- **Given** a mix of pending, accepted, and completed comments,
- **When** `tai list --status accepted --status completed` runs,
- **Then** the output contains both accepted and completed rows,
- **And** pending rows are filtered out.

Exercised by `TestList_TCTRG023_multi_status_or`.

### TC-TRG-025 — `--status` filter narrows the list

- **Given** a mix of pending and accepted comments,
- **When** `tai list --status accepted` runs,
- **Then** only accepted rows appear.

Exercised by `TestList_TCTRG025_single_status_filter`.

### TC-TRG-026 — unknown `--status` value is rejected

- **Given** `--status urgent` is passed,
- **When** the CLI parses,
- **Then** it exits `1` with `TRIAGE_INVALID_FLAGS`.

Exercised by `TestList_TCTRG026_unknown_status`.

### TC-TRG-030 — pending comment has no Resolution / Dismissed sections

- **Given** a pending comment with no `batch_id`,
- **When** `tai show <id>` runs,
- **Then** stdout omits `## Resolution`, `## Dismissed because`, and
  the `**Batch:**` meta line.

Exercised by `internal/cmd/show_test.go` → `TestShow_TCTRG030_pending_comment`.

### TC-TRG-031 — accepted comment with resolution shows `## Resolution`

- **Given** an accepted comment with `resolution = "use execFileSync"`,
- **When** `tai show <id>` runs,
- **Then** stdout contains `## Resolution\nuse execFileSync`.

Exercised by `TestShow_TCTRG031_accepted_with_resolution`.

### TC-TRG-032 — dismissed comment shows `## Dismissed because`

- **Given** a dismissed comment,
- **When** `tai show <id>` runs,
- **Then** stdout contains `## Dismissed because\n<reason> (by <name>)`.

Exercised by `TestShow_TCTRG032_dismissed`.

### TC-TRG-033 — comment with batch shows batch meta line

- **Given** a comment whose `batch_id` references batch `B1` with
  title `Replace execSync`,
- **When** `tai show <id>` runs,
- **Then** the meta lines include `**Batch:** B1 — Replace execSync`.

Exercised by `TestShow_TCTRG033_batch_meta_present`.

### TC-TRG-034 — `tai show --all` joins blocks with `---`

- **Given** a scope with two comments,
- **When** `tai show --all` runs,
- **Then** stdout contains both blocks separated by a line containing
  exactly `---`.

Exercised by `TestShow_TCTRG034_all_two_comments`.

### TC-TRG-035 — `tai show --all` on an empty scope is zero-byte stdout

- **Given** a scope with zero comments,
- **When** `tai show --all` runs,
- **Then** stdout is empty.

Exercised by `TestShow_TCTRG035_all_empty`.

### TC-TRG-036 — `tai show --all --status` filters output

- **Given** a scope with one accepted and one pending comment,
- **When** `tai show --all --status accepted` runs,
- **Then** stdout contains the accepted comment's block,
- **And** the pending comment's block is absent.

Exercised by `TestShow_TCTRG036_all_status_filter`.

### TC-TRG-038 — `--status` is rejected on single-comment `tai show`

- **Given** `tai show <id> --status pending`,
- **When** the CLI parses,
- **Then** it exits `1` with `TRIAGE_INVALID_FLAGS`.

Exercised by `TestShow_TCTRG038_status_rejected_on_single`.

### TC-TRG-040 — `tai accept <id>` transitions to `accepted`

- **Given** a pending comment,
- **When** `tai accept <id> --resolution "..."` runs,
- **Then** the comment's status becomes `accepted` and the resolution
  is persisted (verified via `tai show`).

Exercised by `internal/cmd/mutate_test.go` → `TestAccept_TCTRG040_accept_pending`.

### TC-TRG-041 — accept after dismiss clears the dismissal fields

- **Given** a previously-dismissed comment,
- **When** `tai accept <id>` runs,
- **Then** `dismissed_by` and `dismiss_reason` are cleared.

Exercised by `TestAccept_TCTRG041_reversal_from_dismissed`.

### TC-TRG-042 — accept is idempotent

- **Given** an already-accepted comment,
- **When** `tai accept <id>` runs again with no `--resolution`,
- **Then** the CLI exits `0` and no row changes.

Exercised by `TestAccept_TCTRG042_idempotent`.

### TC-TRG-043 — `tai accept --batch <key>` flips every member

- **Given** a batch with two pending members,
- **When** `tai accept --batch <key>` runs,
- **Then** all members become `accepted`.

Exercised by `TestAccept_TCTRG043_by_batch`.

### TC-TRG-044 — `<id>` and `--batch` are mutually exclusive

- **Given** both `<id>` and `--batch` are supplied,
- **When** the CLI parses,
- **Then** it exits `1` with `TRIAGE_INVALID_FLAGS`.

Exercised by `TestAccept_TCTRG044_mutex`.

### TC-TRG-045 — accept on a non-existent position fails `TRIAGE_NOT_FOUND`

- **Given** the scope has only N comments,
- **When** `tai accept <N+1>` runs,
- **Then** the CLI exits `2` with `TRIAGE_NOT_FOUND`.

Exercised by `TestAccept_TCTRG045_not_found`.

### TC-TRG-050 — `tai dismiss` requires `--reason`

- **Given** `tai dismiss <id>` is invoked without `--reason`,
- **When** the CLI parses,
- **Then** it exits `1` with `TRIAGE_INVALID_FLAGS`.

Exercised by `TestDismiss_TCTRG050_missing_reason`.

### TC-TRG-051 — `--by` override records the dismissed_by attribution

- **Given** `tai dismiss <id> --reason "..." --by alice`,
- **When** the verb runs,
- **Then** the rendered "Dismissed because" line reads `<reason> (by alice)`.

Exercised by `TestDismiss_TCTRG051_records_by`.

### TC-TRG-052 — dismiss-after-accept clears `resolution`

- **Given** a previously-accepted comment with a recorded resolution,
- **When** `tai dismiss <id> --reason "..."` runs,
- **Then** the comment's status becomes `dismissed`,
- **And** the `## Resolution` section is absent from `tai show`.

Exercised by `TestDismiss_TCTRG052_reversal_clears_resolution`.

### TC-TRG-060 — `tai complete <id>` transitions to `completed`

- **Given** a pending comment,
- **When** `tai complete <id> --resolution "..."` runs,
- **Then** the comment's status becomes `completed` and the
  resolution is persisted.

Exercised by `TestComplete_TCTRG060_complete_pending`.

### TC-TRG-070 — uniform-terminal batch becomes that state

- **Given** a batch with two pending members,
- **When** `tai accept --batch <key>` runs (flipping both to accepted),
- **Then** the batch's status becomes `accepted` (the uniform
  terminal state of its members).

Exercised by `TestBatch_TCTRG070_uniform_terminal`.

### TC-TRG-071 — split batch becomes `mixed`

- **Given** a batch whose two members transition to different
  terminal states (`accepted` + `dismissed`),
- **When** `tai status` runs,
- **Then** the batch row reads `(<n> comments — mixed)`.

Exercised by `TestBatch_TCTRG071_split_is_mixed`.

### TC-TRG-072 — batch with pending + terminal members is `mixed`

- **Given** a batch with two members, one accepted and one pending,
- **When** `tai status` runs,
- **Then** the batch's status reads `mixed`.

Exercised by `TestBatch_TCTRG072_pending_plus_terminal_is_mixed`.

### TC-TRG-080 — `tai status` summary for a PR scope with batches

- **Given** a PR scope with one comment in one batch,
- **When** `tai status` runs,
- **Then** stdout contains `Repo:`, `Scope:`, the counts block, and a
  `Batches:` block with one batch entry.

Exercised by `internal/cmd/status_test.go` → `TestStatus_TCTRG080_pr_with_batches`.

### TC-TRG-081 — `tai status` omits the Batches block when zero batches

- **Given** a branch scope with comments but no batches,
- **When** `tai status` runs,
- **Then** the output contains the counts block and does NOT contain
  the `Batches:` line.

Exercised by `TestStatus_TCTRG081_branch_without_batches`.

### TC-TRG-082 — `tai status` empty scope reads `Total: 0`

- **Given** a target with zero comments,
- **When** `tai status` runs,
- **Then** stdout contains `Total:      0` and the per-status lines
  are suppressed.

Exercised by `TestStatus_TCTRG082_empty_scope`.

### TC-TRG-090 — `tai forget` with no selector fails

- **Given** `tai forget` is invoked with no `--comment`/`--batch`/
  `--pr`/`--branch`/`--repo`,
- **When** the CLI parses,
- **Then** it exits `1` with `TRIAGE_INVALID_FLAGS`.

Exercised by `internal/cmd/forget_test.go` → `TestForget_TCTRG090_zero_selectors`.

### TC-TRG-091 — multiple local selectors are rejected

- **Given** `tai forget --pr 1 --branch feat/x --yes`,
- **When** the CLI parses,
- **Then** it exits `1` with `TRIAGE_INVALID_FLAGS`.

Exercised by `TestForget_TCTRG091_two_local_selectors`.

### TC-TRG-092 — `tai forget --repo <owner/name> --yes` works outside any git repo

- **Given** the working directory is not in any git repo and the
  given repo has been imported,
- **When** `tai --repo <owner/name> forget --yes` runs,
- **Then** the destructive summary is printed,
- **And** `Done.` is printed,
- **And** the CLI exits `0`,
- **And** a follow-up `tai list --pr 1` exits `TRIAGE_NOT_FOUND`
  (the cascade removed the repo and every child row).

Exercised by `TestForget_TCTRG092_repo_with_yes_outside_git`.

### TC-TRG-093 — non-interactive without consent fails loudly

- **Given** stdin is non-TTY and neither `--yes` nor a truthy
  `TAI_ACCEPT_DESTRUCTIVE` is set,
- **When** `tai forget --pr <N>` runs,
- **Then** the CLI exits `1` with `TRIAGE_CONFIRMATION_REQUIRED`,
- **And** no rows are deleted.

Exercised by `TestForget_TCTRG093_non_interactive_no_consent`.

### TC-TRG-094 — `TAI_ACCEPT_DESTRUCTIVE=1` grants consent

- **Given** the env var is set to a truthy value,
- **When** `tai forget --pr <N>` runs non-interactively,
- **Then** the CLI exits `0`.

Exercised by `TestForget_TCTRG094_env_skips_prompt`.

### TC-TRG-095 — `--status` prunes matching comments only

- **Given** a PR scope where one comment is `completed` and another
  is `pending`,
- **When** `tai forget --pr <N> --status completed --yes` runs,
- **Then** the completed comment is deleted,
- **And** the pending comment survives,
- **And** the PR row is preserved.

Exercised by `TestForget_TCTRG095_status_prune_pr`.

### TC-TRG-096 — `--status` combined with `--comment` is rejected

- **Given** `tai forget --comment 1 --status completed`,
- **When** the CLI parses,
- **Then** it exits `1` with `TRIAGE_INVALID_FLAGS`.

Exercised by `TestForget_TCTRG096_status_on_comment_rejected`.

### TC-TRG-097 — `tai forget --repo --status` prunes matching comments repo-wide

- **Given** two PRs each with one completed comment under the same repo,
- **When** `tai --repo <owner/name> forget --status completed --yes` runs,
- **Then** every completed comment is deleted,
- **And** the PR rows are preserved (`tai list --pr <N>` returns
  `(no comments)`).

Exercised by `TestForget_TCTRG097_repo_status_prune`.

### TC-TRG-098 — `tai forget --batch --status` recomputes batch status

- **Given** a batch with one completed and one accepted member,
- **When** `tai forget --pr <N> --batch B1 --status completed --yes` runs,
- **Then** the completed member is deleted,
- **And** the batch row survives,
- **And** the batch's status recomputes to `accepted` (the surviving
  member's status).

Exercised by `TestForget_TCTRG098_batch_status_recompute`.

### TC-TRG-099 — multiple `--status` values combine via OR

- **Given** a PR with one completed, one dismissed, and one pending comment,
- **When** `tai forget --pr <N> --status completed --status dismissed --yes` runs,
- **Then** the completed and dismissed comments are deleted,
- **And** the pending comment survives.

Exercised by `TestForget_TCTRG099_multi_value_status`.

### TC-TRG-100 — `[exit 2: TRIAGE_NO_SCOPE]` footer

- **Given** a verb that cannot resolve a scope (no `--pr`/`--branch`,
  not in a git repo with imported data),
- **When** the verb runs,
- **Then** the rendered stderr footer is `[exit 2: TRIAGE_NO_SCOPE]`.

Exercised by `internal/cmd/errcode_triage_test.go` →
`TestErrcode_TCTRG100_no_scope_footer`.

### TC-TRG-101 — `[exit 2: TRIAGE_AMBIGUOUS_SCOPE]` footer

- **Given** the current branch matches both a PR's `head_branch` and a
  `branches.name` row,
- **When** a triage verb runs without explicit scope,
- **Then** the rendered stderr footer is
  `[exit 2: TRIAGE_AMBIGUOUS_SCOPE]`.

Exercised by `TestErrcode_TCTRG101_ambiguous_scope_footer`.

### TC-TRG-102 — `[exit 2: TRIAGE_NOT_FOUND]` footer

- **Given** a comment / batch / PR / branch referenced by a verb does
  not exist,
- **When** the verb runs,
- **Then** the rendered stderr footer is `[exit 2: TRIAGE_NOT_FOUND]`.

Exercised by `TestErrcode_TCTRG102_not_found_footer`.

### TC-TRG-103 — `[exit 1: TRIAGE_INVALID_FLAGS]` footer

- **Given** a triage verb is invoked with conflicting flags,
- **When** the CLI parses,
- **Then** the rendered stderr footer is `[exit 1: TRIAGE_INVALID_FLAGS]`.

Exercised by `TestErrcode_TCTRG103_invalid_flags_footer`.

### TC-TRG-104 — `[exit 1: TRIAGE_CONFIRMATION_REQUIRED]` footer

- **Given** `tai forget` is invoked non-interactively without `--yes`
  or `TAI_ACCEPT_DESTRUCTIVE`,
- **When** the verb runs,
- **Then** the rendered stderr footer is
  `[exit 1: TRIAGE_CONFIRMATION_REQUIRED]`.

Exercised by `TestErrcode_TCTRG104_confirmation_required_footer`.

<!-- Coverage note: the interactive `[y/N]` consent path in
`consentGranted` (forget.go) is not E2E-testable via `cmdtest.Run`
because the harness wires a `strings.Reader` for stdin (always
non-TTY). The non-interactive path is covered by TC-TRG-093 / TC-TRG-104;
the env-var and --yes paths are covered by TC-TRG-094 / TC-TRG-092. The
interactive `y`/`Y` branch is exercised only manually. -->

