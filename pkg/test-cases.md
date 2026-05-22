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
