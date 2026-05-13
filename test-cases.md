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

(`/tai:triage` and `/tai:verify` slash commands are exercised manually —
they have no TC-IDs because they aren't unit-testable from Go.)

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

<!-- Reserved for the storage proposal (add-storage-schema). Cases will
cover the migration runner, every table's constraints, cascade rules, and
the storage-layer error codes. -->

---

## INST — install / uninstall

<!-- Reserved for the install proposal (add-install-command). Cases will
cover the file-state classifier (missing / up-to-date / stale-but-untouched
/ user-modified), the prompt UX, --force, TAI_ACCEPT_COMMAND_UPDATES, and
the uninstall mirror. -->

---

## IMP — import

<!-- Reserved for the import proposal (add-import-command). Cases will
cover JSON-payload validation, repo+target upsert, external_refs key
resolution, the frozen-on-import rule, batch upsert, and the empty-payload
success path. -->

---

## TRG — triage state

<!-- Reserved for the triage-state proposal (add-triage-state). Cases will
cover scope resolution (PR / branch / auto-detect / ambiguity), per-target
ID translation, list / show with --status filters, accept / dismiss /
complete state transitions and batch operations, status, forget (including
--status modifier for bulk prune), and the consent model. -->
