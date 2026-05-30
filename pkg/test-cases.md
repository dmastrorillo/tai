# test-cases.md — pkg/ framework behavioural specification

This file is the authoritative, human-readable specification of how the
shared framework packages under `pkg/` behave: the error-template writer
(`pkg/cliout`), the panic-recovery wrapper (`pkg/cliexec`), the
error-code taxonomy (`pkg/errcode`), and the exit-code constants
(`pkg/exitcode`).

`pkg/` is on a public stability contract: append-only error codes, no
renaming or repurposing exported symbols. The cases below are the
guarantees plugin authors (first- and third-party) get to rely on.

The flow is: **OpenSpec proposal → BDD cases here → tests → production
code → observed behaviour.** A change is "real" only after it appears
as a Given / When / Then below, is exercised by a test that names its
TC-ID, and is implemented behind that test.

Core-CLI behaviour lives in [`core/test-cases.md`](../core/test-cases.md);
the triage plugin's behaviour lives in
[`plugins/triage/test-cases.md`](../plugins/triage/test-cases.md) (each
plugin under `plugins/` ships its own `test-cases.md`). See
[`CLAUDE.md`](../CLAUDE.md) for the full pipeline and ID-scheme rules.

---

## Categories

Each case has an ID of the form `TC-<CATEGORY>-<NUMBER>`. Categories are
short, stable codes; numbers increment within each category starting at
`001`, zero-padded to 3 digits, and remain globally unique across
components. **Never renumber existing IDs.**

| Code | Scope |
|------|-------|
| [`ERR`](#err--error-contract) | Error-message template, error-code taxonomy, exit-code mapping, panic recovery |
| [`CFG`](#cfg--data-directory-resolution) | `pkg/datadir`: data-directory resolution, env-var precedence, unwritable-dir error |

---

## ERR — error contract

### TC-ERR-001 — panic during command execution surfaces as `INTERNAL_ERROR`

- **Given** any tai command whose action panics,
- **When** `cliexec.Run` invokes that command,
- **Then** the returned error is a `*errcode.Error` with `Code =
  INTERNAL_ERROR`,
- **And** the message embeds the recovered panic value,
- **And** the mapped exit code is `70`.

Exercised by `pkg/cliexec/cliexec_test.go` →
`TestRun_TCERR001_panic_becomes_internal_error`.

### TC-ERR-002 — full error template renders for an `*errcode.Error` with help

- **Given** a `*errcode.Error` constructed with a code, message, and one
  or more help bullets,
- **When** `cliout.WriteError` formats it,
- **Then** the output contains an `Error: <msg>` line, a `What to do:`
  block with one bullet per help entry, and a final `[exit N: CODE]`
  footer on its own line.

Exercised by `pkg/cliout/cliout_test.go` →
`TestWriteError_TCERR002_template_with_help`.

### TC-ERR-003 — `INTERNAL_ERROR` with no help bullets omits the "What to do" block

- **Given** a `*errcode.Error` with `Code = INTERNAL_ERROR` and no help
  bullets (the recovered-panic shape),
- **When** `cliout.WriteError` formats it,
- **Then** the output contains the `Error:` line and the `[exit 70:
  INTERNAL_ERROR]` footer,
- **And** the output does NOT contain a `What to do:` block.

Exercised by `pkg/cliout/cliout_test.go` →
`TestWriteError_TCERR003_internal_error_omits_help`.

### TC-ERR-004 — footer regex invariant across every error path

- **Given** any error written via `cliout.WriteError`,
- **When** the output is inspected,
- **Then** the last non-empty line matches
  `^\[exit \d+: [A-Z][A-Z0-9_]*\]$`.

The test implementation iterates over every `errcode.Code` in the
taxonomy (`allCodes` slice) — adding a new code to `pkg/errcode`
requires updating that slice in the same change, or the new code is
not actually covered by this invariant. As of Phase 2 the slice
covers the foundation codes, the Phase 1 `CONFIG_*` /
`TAI_NOT_CONFIGURED` / `MISSING_ARG` codes, the Phase 2 `REPO_FETCH_FAILED`
/ `REPO_INIT_TARGET_NOT_EMPTY` / `REPO_INIT_GIT_UNAVAILABLE` codes,
and the storage / install / import / triage layer codes.

Exercised by `pkg/cliout/cliout_test.go` →
`TestWriteError_TCERR004_footer_regex_invariant`.

### TC-ERR-005 — panic with an error value preserves the cause chain

- **Given** an action panics with a value that is itself an `error`
  (a sentinel returned from a helper, for instance),
- **When** `cliexec.Run` recovers,
- **Then** the returned error is a `*errcode.Error{Code: INTERNAL_ERROR}`,
- **And** `errors.Is(returned, sentinel)` is true,
- **And** `errors.Unwrap(returned)` returns the sentinel.

Exercised by `pkg/cliexec/cliexec_test.go` →
`TestRun_TCERR005_panic_with_error_preserves_cause_chain`.

### TC-ERR-006 — normal action errors pass through unchanged

- **Given** an action returns a non-nil error WITHOUT panicking,
- **When** `cliexec.Run` returns,
- **Then** the returned error is the same value the action produced
  (verified via `errors.Is`).

Exercised by `pkg/cliexec/cliexec_test.go` →
`TestRun_TCERR006_passes_through_non_panic_errors`.

### TC-ERR-007 — unstructured errors render as `INTERNAL_ERROR`

- **Given** an arbitrary `error` value that is NOT a `*errcode.Error`,
- **When** `cliout.WriteError` formats it,
- **Then** stderr starts with `Error: <msg>` (the original message),
- **And** the footer is `[exit 70: INTERNAL_ERROR]`.

Exercised by `pkg/cliout/cliout_test.go` →
`TestWriteError_TCERR007_unstructured_error_becomes_internal_error`.

### TC-ERR-008 — error messages with embedded newlines collapse to one line

- **Given** an error whose `Error()` returns multiple newline-separated
  lines,
- **When** `cliout.WriteError` formats it,
- **Then** the rendered `Error:` line is a single line containing all
  the original content, separated by spaces (no embedded newlines),
- **And** the footer remains the last line of stderr.

Exercised by `pkg/cliout/cliout_test.go` →
`TestWriteError_TCERR008_multiline_message_collapsed_to_single_line`.

---

## CFG — data directory resolution

The `pkg/datadir` package owns the resolution and (lazy) creation of
TAI's per-user data directory. Promoted from
`plugins/triage/internal/datadir` in Phase 2 of pivot-to-ai-as-code so
both `core/internal/sync` and the triage plugin can import it. The
TC-IDs were established when the package lived under triage; per the
never-renumber rule they stay TC-CFG-* across the move.

### TC-CFG-001 — default on Linux with no overrides

- **Given** `$XDG_DATA_HOME` and `$TAI_DATA_DIR` are both unset and
  `$HOME` is `/tmp/fake-home`,
- **When** `datadir.Resolve()` runs on Linux/macOS,
- **Then** the resolved path is `/tmp/fake-home/.local/share/tai`.

On Windows, the equivalent: with `LOCALAPPDATA` set to
`C:\Users\test\AppData\Local`, the resolved path is
`C:\Users\test\AppData\Local\tai`.

Exercised by `pkg/datadir/datadir_test.go` →
`TestResolve_TCCFG001_default_linux_no_overrides`.

### TC-CFG-002 — `XDG_DATA_HOME` overrides the OS default

- **Given** `$XDG_DATA_HOME` is `/custom/xdg` and `$TAI_DATA_DIR` is
  unset,
- **When** `datadir.Resolve()` runs,
- **Then** the resolved path is `/custom/xdg/tai` (the literal `tai`
  suffix is appended).

Exercised by `pkg/datadir/datadir_test.go` →
`TestResolve_TCCFG002_xdg_overrides_default`.

### TC-CFG-003 — `TAI_DATA_DIR` wins over `XDG_DATA_HOME` and is used verbatim

- **Given** `$TAI_DATA_DIR` is `/explicit/tai-data` and `$XDG_DATA_HOME`
  is `/custom/xdg`,
- **When** `datadir.Resolve()` runs,
- **Then** the resolved path is `/explicit/tai-data` exactly — no `tai`
  suffix is appended.

Exercised by `pkg/datadir/datadir_test.go` →
`TestResolve_TCCFG003_tai_data_dir_wins`.

### TC-CFG-004 — unwritable data directory surfaces `DATA_DIR_UNWRITABLE`

- **Given** `$TAI_DATA_DIR` points at a path that cannot be created
  (e.g. `/dev/null/cannot-mkdir`),
- **When** `datadir.EnsureWritable()` runs,
- **Then** the returned error is a `*errcode.Error` with
  `Code = DATA_DIR_UNWRITABLE`,
- **And** the error carries at least one remediation help bullet,
- **And** the error message names the failing path.

Exercised by `pkg/datadir/datadir_test.go` →
`TestEnsureWritable_TCCFG004_unwritable_dir`.

Integration coverage of the real-filesystem read-only path (chmod 555)
lives in `datadir_integration_test.go` behind the `integration` build tag.
