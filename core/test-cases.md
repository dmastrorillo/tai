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
