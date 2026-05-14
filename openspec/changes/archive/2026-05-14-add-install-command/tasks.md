# Implementation Tasks

> **Prerequisite (out of pipeline):** `add-tai-foundation` is implemented (`internal/cmdframework` exposes hash + frontmatter helpers and the `Ledger(verb)` accessor with a stub population).
>
> Add BDD cases under the `INST` category (new — add to `test-cases.md` ToC if absent) before each TDD slice.

## 1. Hash ledger storage layer

- [x] 1.1 Define the ledger file format at `commands/<verb>.ledger.json` (array of `sha256:<hex>` strings, oldest-first)
- [x] 1.2 Replace the foundation's stub `cmdframework.Ledger(verb)` with a real `//go:embed`-backed implementation reading from `commands/*.ledger.json`
- [x] 1.3 TC-INST-001..003: ledger shape (well-formed array, hex regex, oldest-first ordering), missing file returns empty slice, current build's hash is the last entry
- [x] 1.4 Build-time test that fails if any `commands/<verb>.md` body hash does not equal the last entry of its ledger

## 2. Build-time ledger helper (`cmd/tai-ledger`)

- [x] 2.1 Implement `cmd/tai-ledger/main.go`: iterate `commands/*.md`, compute body hashes, append missing hashes to the matching `commands/<verb>.ledger.json` (creating the file if absent)
- [x] 2.2 Make the helper idempotent — running twice with no body changes is a no-op
- [x] 2.3 Add a Makefile target `make ledger-update` and document it in the package README
- [x] 2.4 Unit tests covering: new command (file created), unchanged command (no-op), changed command (one entry appended), malformed existing ledger (clear error)

## 3. File-state classifier

- [x] 3.1 Implement `internal/installer.Classify(verb string, target string) (Classification, error)` returning one of `missing`, `up-to-date`, `stale-but-untouched`, `user-modified`
- [x] 3.2 TC-INST-010..013: one TC per classification (file missing, body hash matches current, body hash in ledger but stale, body hash absent from ledger)
- [x] 3.3 Edge case: file exists but is unparseable as frontmatter+body — classify as `user-modified` (most conservative)

## 4. `tai install` command

- [x] 4.1 Wire `tai install` subcommand into the urfave/cli app; mark as repo-independent
- [x] 4.2 Implement flags: `--commands-dir`, `--force`
- [x] 4.3 Resolve default target directory cross-platform via `os.UserHomeDir`
- [x] 4.4 For each bundled command: classify → decide action → execute (write / overwrite / skip / prompt)
- [x] 4.5 Implement the prompt: read from stdin, default `N` on empty input, accept `y` or `Y` for yes
- [x] 4.6 Detect non-interactive stdin via `term.IsTerminal(syscall.Stdin)`; suppress prompt and record `prompted-skipped`
- [x] 4.7 Honour `TAI_ACCEPT_COMMAND_UPDATES` env var with truthy semantics: case-insensitive match against `1`, `true`, `yes`, `on`, `y`, `t` enables overwrite (same effect as `--force`). All other values (including unset, empty, `0`, `false`, `no`, `off`, unrecognised strings) are treated as off. Implement via a small helper `isTruthyEnv(name string) bool` reused for any future env-var toggles.
- [x] 4.8 Emit the summary block per the spec
- [x] 4.9 Map errors to `INSTALL_TARGET_UNWRITABLE`, `INSTALL_INVALID_TARGET`, `INSTALL_LEDGER_CORRUPT`
- [x] 4.10 End-to-end tests: TC-INST-020..029 covering fresh install, idempotent re-run, classify each file state, `--force`, env var override, env var with non-`1` value ignored, `--commands-dir`, unwritable target, malformed `--commands-dir`, non-interactive stdin without override

## 5. `tai uninstall` command

- [x] 5.1 Wire `tai uninstall` subcommand; mark as repo-independent
- [x] 5.2 Implement flags: `--commands-dir`, `--force`
- [x] 5.3 Walk the target directory; for each filename matching a known verb, classify and remove (or skip on user-modified without `--force` / env var)
- [x] 5.4 Leave files whose filename matches no known verb in place
- [x] 5.5 Remove the target directory iff it is empty after processing
- [x] 5.6 Honour `TAI_ACCEPT_COMMAND_UPDATES=1` for uninstall identically to install
- [x] 5.7 Emit the summary block with `Removed`, `Prompted-skipped`, `Not-found` buckets
- [x] 5.8 End-to-end tests: TC-INST-030..036 covering clean uninstall, leave-modified-in-place, `--force` removes modified, env var removes modified, unrelated files preserved, dir preserved when non-empty, dir removed when empty

## 6. Repo-independence and integration

- [x] 6.1 Test that `tai install` and `tai uninstall` succeed when run from outside any git repository (no `REPO_NOT_FOUND`)
- [x] 6.2 Test that running both commands in a foreign data-directory context (`TAI_DATA_DIR=/tmp/x`) leaves the data directory untouched (install does NOT create the SQLite database)

## 7. Documentation and validation

- [x] 7.1 Add `INST` to `test-cases.md` ToC
- [x] 7.2 Author short package doc in `internal/installer` pointing at `specs/install/spec.md`
- [x] 7.3 Extend `internal/errcode` with the three new codes; ensure the in-code taxonomy table matches the spec
- [x] 7.4 `go test ./... && go vet ./... && gofmt -l .` clean; `go test -race ./...` clean
- [x] 7.5 `openspec validate add-install-command` reports no errors
