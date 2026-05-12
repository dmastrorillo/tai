# Implementation Tasks

> **Prerequisite (out of pipeline):** Project bootstrap is complete — `cmd/tai/main.go` exists, `go.mod` is initialised, `urfave/cli` is wired, `test-cases.md` exists with its category table of contents, and `tai --version` runs end-to-end as a walking skeleton.
>
> Each numbered task below corresponds to a TDD slice. Before writing code for a task, add the matching BDD case(s) to `test-cases.md` under the appropriate category and assign a TC-ID; then write the failing test naming that TC-ID; then implement.

## 1. Error formatting and exit codes

- [ ] 1.1 Add `test-cases.md` category `ERR` (error contract) to the ToC if absent
- [ ] 1.2 Add BDD cases TC-ERR-001..00N for the scenarios in `specs/cli-framework/spec.md` under "Human-readable error contract" (footer format, "What to do" block, last-line invariant, optional remediation for internal errors)
- [ ] 1.3 Implement `internal/cliout` package: function that writes the standard error template (`Error:` line, optional "What to do:" block, `[exit N: CODE]` footer) to stderr
- [ ] 1.4 Implement exit-code enum (`internal/exitcode`): named constants `Success=0`, `Usage=1`, `Precondition=2`, `Data=3`, `Internal=70`
- [ ] 1.5 Implement error-code taxonomy (`internal/errcode`): exported `Code` type with the five seed codes from the spec (`REPO_NOT_FOUND`, `REPO_FLAG_INVALID`, `DATA_DIR_UNWRITABLE`, `UNKNOWN_SUBCOMMAND`, `INTERNAL_ERROR`); a method that returns the exit code for a given error code
- [ ] 1.6 Wire `cmd/tai/main.go` so the only place that translates errors into exit codes is `main` (per `CLAUDE.md` convention)
- [ ] 1.7 End-to-end test: invoking `tai` with an unknown subcommand emits the standard error footer with code `UNKNOWN_SUBCOMMAND` and exit `1`
- [ ] 1.8 End-to-end test: recovered panic surfaces as `INTERNAL_ERROR` with exit `70` and no "What to do:" block required

## 2. Data directory resolution

- [ ] 2.1 Add `test-cases.md` category `CFG` (configuration / data dir) to the ToC if absent
- [ ] 2.2 Add BDD cases TC-CFG-001..004 for the data-directory scenarios in `specs/cli-framework/spec.md` (default on Linux, XDG override, TAI_DATA_DIR override, unwritable directory)
- [ ] 2.3 Implement `internal/datadir` resolver with the documented precedence: `TAI_DATA_DIR` → `XDG_DATA_HOME` → OS-default
- [ ] 2.4 Resolver MUST NOT create the directory at resolution time; provide a separate `EnsureWritable(ctx)` that creates lazily and returns `DATA_DIR_UNWRITABLE` on failure
- [ ] 2.5 Unit tests for the resolver covering each precedence rung, including Windows (`%LOCALAPPDATA%`) — gated behind `runtime.GOOS` checks where applicable
- [ ] 2.6 Integration test (build tag `integration`): `EnsureWritable` against a tmp dir, against a read-only dir (using `os.Chmod`)

## 3. Repo identity detection

- [ ] 3.1 Add `test-cases.md` category `REPO` (repo context) to the ToC if absent
- [ ] 3.2 Add BDD cases TC-REPO-001..006 for the scenarios in `specs/cli-framework/spec.md` under "Repo identity detection" and "Global --repo override flag" (SSH URL, HTTPS with `.git`, HTTPS without `.git`, not a repo, no origin, `--repo` override, malformed `--repo`)
- [ ] 3.3 Implement `internal/repoctx` parser: pure function that takes an `origin` URL string and returns `owner/name` or an error, accepting SSH and HTTPS forms
- [ ] 3.4 Implement `internal/repoctx` reader: invokes `git config --get remote.origin.url` in the working directory via `os/exec`, returns parsed identity or a typed error indicating "not a repo" vs "no origin"
- [ ] 3.5 Wire global `--repo <owner/name>` flag on the root `urfave/cli` app; flag value, when present, bypasses detection
- [ ] 3.6 Validate `--repo` value against `<owner>/<name>` (no slash in owner or name; non-empty); on failure, exit `1` with `REPO_FLAG_INVALID`
- [ ] 3.7 Provide a `Resolver` helper that subcommands call when they need repo context; returns identity or surfaces `REPO_NOT_FOUND` / `REPO_FLAG_INVALID`
- [ ] 3.8 End-to-end test: a command marked as repo-requiring fails with `REPO_NOT_FOUND` exit `2` outside a git repo when no `--repo` is given
- [ ] 3.9 End-to-end test: the same command succeeds when `--repo acme/app` is provided outside a git repo

## 4. Repo-independent commands carve-out

- [ ] 4.1 Add BDD cases TC-CMD-001 (`tai --version` outside repo) and TC-CMD-002 (`tai --help` outside repo) to `test-cases.md`
- [ ] 4.2 Mark `--help` and `--version` as repo-independent in the `urfave/cli` wiring; ensure they do not invoke the repo resolver
- [ ] 4.3 Provide a mechanism (struct tag, builder option, or table) for future subcommands to declare themselves repo-independent
- [ ] 4.4 End-to-end test: `tai --help` and `tai --version` exit `0` from a tmp directory that is not a git repo

## 5. Slash-command frontmatter schema and hashing

- [ ] 5.1 Add `test-cases.md` category `CMD` (command framework / slash commands) to the ToC if absent
- [ ] 5.2 Implement `internal/cmdframework` frontmatter parser: reads a slash-command markdown, returns a struct with the six required fields, errors on missing/extra fields
- [ ] 5.3 Implement `internal/cmdframework` body extractor: returns the byte slice after the closing `---` line (inclusive of trailing newline if present, exclusive of the `---` line itself)
- [ ] 5.4 Implement `internal/cmdframework` hash function: sha256 over the body bytes, formatted as `sha256:<hex>`
- [ ] 5.5 Unit tests covering frontmatter validation (golden good frontmatter, missing `content_hash`, unknown extra field), body extraction (with and without trailing newline), hash determinism
- [ ] 5.6 Build-time check (Makefile target or `go generate` hook): for every bundled `commands/*.md`, recompute the hash and assert it equals the value in the frontmatter; fail the build on mismatch

## 6. Embedded hash ledger (foundation portion)

- [ ] 6.1 Define `internal/cmdframework`'s contract for the embedded ledger: a function `Ledger(verb string) []string` returning all historical content hashes for a verb, ordered oldest-first
- [ ] 6.2 Provide a stub ledger source (an empty per-verb list) that subsequent proposals (`add-install-command`) replace with a real `//go:embed`'d JSON or similar
- [ ] 6.3 Unit test: `Ledger("import")` for a verb that has never shipped returns an empty slice without error
- [ ] 6.4 Document in `internal/cmdframework/README.md` (or package doc) that the ledger format and population is owned by the install capability; foundation only defines the access contract

## 7. Cross-cutting and hygiene

- [ ] 7.1 Add a `tai` smoke test that exercises the error footer regex `^\[exit \d+: [A-Z][A-Z0-9_]*\]$` for every standard error path introduced in this change
- [ ] 7.2 Update `CLAUDE.md` if any new project conventions emerge (e.g. new test-cases.md categories `ERR`, `CFG`, `REPO`, `CMD`) — keep the ToC there in sync with `test-cases.md`
- [ ] 7.3 `go test ./... && go vet ./... && gofmt -l .` clean; `go test -race ./...` clean before requesting archive
- [ ] 7.4 `openspec validate add-tai-foundation` reports no errors before requesting archive
