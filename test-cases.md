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

<!-- Add new CMD cases here as their proposals land. -->

---

## ERR — error contract

<!-- Reserved for the foundation proposal (add-tai-foundation). Cases will
cover the error-message template (Error line, "What to do" block, footer),
the error-code taxonomy, and exit-code mapping. -->

---

## CFG — data directory & config resolution

<!-- Reserved for the foundation proposal. Cases will cover the
TAI_DATA_DIR / XDG_DATA_HOME / OS-default precedence and the lazy-create
behaviour. -->

---

## REPO — repo-context detection

<!-- Reserved for the foundation proposal. Cases will cover origin URL
parsing (SSH / HTTPS / with-or-without .git), the --repo flag, and the
auto-detect path including the "not a repo" and "no origin" errors. -->

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
