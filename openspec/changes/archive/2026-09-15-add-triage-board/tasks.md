## 1. BDD cases — add TC-IDs before any code

Section 8 assigns each group below to the slice that implements it; the entry and the tests naming it always land in the same diff.

- [x] 1.1 In `plugins/triage/test-cases.md`, add a new `BRD` category to the ToC table with scope "the triage board: loopback server, rendered content, intents artifact, `board intents`".
- [x] 1.2 Pin `TC-BRD-*` IDs for the briefing: `-` with valid JSON serves the board and opens no database; a missing or non-`-` positional exits `UNKNOWN_SUBCOMMAND`; malformed JSON exits `TRIAGE_BOARD_INVALID_JSON`; three simultaneous schema violations are all reported with JSON-Pointer paths under one `TRIAGE_BOARD_SCHEMA_INVALID`; a failed validation binds no listener and launches no browser.
- [x] 1.3 Pin `TC-BRD-*` IDs for the server: loopback bind on an ephemeral port with the URL on stdout; request without the path prefix returns `404` carrying no comment content; browser-launch failure keeps the server running; submit writes the artifact and exits `0` with no database mutation; bind failure exits `TRIAGE_BOARD_UNAVAILABLE`.
- [x] 1.4 Pin `TC-BRD-*` IDs for rendered content: all seven presentation fields appear for a briefed comment; `suggested_fix_origin` is visible; batch members grouped under key and title with both a batch-level and per-member control; batches precede non-batched comments; a note input is present for every comment and every batch; an empty briefing renders its message and offers submit of an empty intent set. Each of these asserts against the served HTML, so each is reachable from a handler test.
- [x] 1.5 Pin `TC-BRD-*` IDs for the artifact: all three intents recorded; note on an `unanswered`; batch call expands to per-member entries with no batch-level entry; resubmission replaces; scope-keyed paths do not cross-read.
- [x] 1.6 Pin `TC-BRD-*` IDs for `board intents`: markdown emission with timestamp and one line per entry in presentation order; `TRIAGE_NO_INTENTS` when absent; repeated reads are identical and non-destructive.
- [x] 1.7 Pin `TC-BRD-*` IDs for the two error codes rendering through the foundation template with the correct `[exit N: CODE]` footer.
- [x] 1.8 Add a note in the same file recording exactly what the board leaves to manual verification: the served HTML, the intents artifact and `tai triage board intents` output all carry TC-BRD IDs and Go tests; what a click does in the page once that HTML is loaded does not. State the boundary explicitly rather than by analogy to the slash commands, whose note covers conversational contracts that carry no TC-IDs at all — the board's exemption is the narrower one.
- [x] 1.9 Cross-check every new TC-ID drives at the boundary the user observes, per CLAUDE.md's north-star rule. A TC about stdout gets a test capturing stdout; a TC about the artifact gets a test inspecting the written file; a TC about an HTTP response gets a handler test.

## 2. Error codes

- [x] 2.1 In `pkg/errcode/errcode.go`, append `TriageBoardInvalidJSON`, `TriageBoardSchemaInvalid`, `TriageBoardUnavailable` and `TriageNoIntents` to the triage-layer block. Append-only — do not reorder or repurpose existing codes.
- [x] 2.2 Map exit codes: `TriageBoardInvalidJSON` → `exitcode.Usage` (`1`) and `TriageBoardSchemaInvalid` → `exitcode.Data` (`3`), matching `ImportInvalidJSON` and `ImportSchemaInvalid`; `TriageBoardUnavailable` → `exitcode.Data` (`3`), alongside `DataDirUnwritable` and the other environment-blocked codes; `TriageNoIntents` → `exitcode.Precondition` (`2`), alongside `TriageNotFound`.
- [x] 2.3 Add the two codes to `pkg/test-cases.md`'s error-code taxonomy section if it enumerates codes individually.

## 3. Briefing decode and validation

- [x] 3.1 Add the briefing's Go types in `plugins/triage/internal/board/`: repo, scope (`pr` or `branch`), batches, and comments carrying `id`, optional `batch_key`, `severity`, and the seven presentation fields plus `suggested_fix_origin`. Reject unknown fields.
- [x] 3.2 Add a validator collecting EVERY violation before returning, each with a JSON-Pointer-style path, mirroring `plugins/triage/internal/import/payload`. Cover: missing or empty required field, unknown field, bad `severity`, bad `scope.kind`, bad `suggested_fix_origin`, and a `batch_key` naming a batch absent from `batches`.
- [x] 3.3 Order the decoded comments for presentation: batches first by highest-severity member then batch key ascending, then non-batched by severity then `id` ascending. Same order the loop uses, so board and conversation present one queue.
- [x] 3.4 Unit-test the validator against a fixture with three simultaneous violations, asserting all three paths appear; and the ordering including both tie-breaks.

## 4. Intents artifact

- [x] 4.1 Create `plugins/triage/internal/board/` with the artifact's Go types: repo, scope, `submitted_at` (RFC 3339), and entries of `{id, intent, note}` where intent is `accept` | `dismiss` | `unanswered`.
- [x] 4.2 Implement the scope-keyed path under `<TAI_DATA_DIR>/plugins/triage/state/intents/` as `<owner>--<name>--pr-<number>.json` or `<owner>--<name>--branch-<slug>.json`. Slug branch names so a `/` in a branch cannot escape the directory.
- [x] 4.3 Implement read and write. Write replaces the file in full. Read returns a typed "absent" result rather than an error so callers distinguish absence from an unreadable file.
- [x] 4.4 Test round-tripping, replacement on resubmit, path derivation for both scope kinds, and that a branch name containing `/` produces a path inside the intents directory.

## 5. Board server and page

- [x] 5.1 Add `plugins/triage/internal/cmd/board.go` wiring `tai triage board -` and register it in `plugins/triage/internal/cmd/root.go`. It takes no scope flags — the briefing's `repo` and `scope` name the artifact, and the shared rule reaches for git and the database, which this verb may not touch. Read and validate stdin before anything else; a rejected briefing must not reach the listener.
- [x] 5.2 Bind `127.0.0.1:0` through a package-level `var listen = net.Listen`. On bind failure return `TRIAGE_BOARD_UNAVAILABLE`. Read the assigned port back from the listener and build the URL from it — never assume a port.
- [x] 5.3 Add `ListenFailureForTesting(t testing.TB, err error)` swapping `listen` for one that returns `err`, restoring it via `t.Cleanup`. Follows the established seam pattern — `config.AllowFileURLsForTesting`, `plugins.RegisterForTesting`, `sync.AutoInstallForTesting` — where the `testing.TB` parameter makes production use a code-review red flag. A kernel-assigned port has no deterministic way to fail, so this seam is the only route to the bind-failure case, and that case is what pins `TRIAGE_BOARD_UNAVAILABLE`'s exit code against an append-only registry. Its test asserts the error code, the exit code, and that no intents artifact is written.
- [x] 5.4 Generate a 32-hex path prefix per invocation from `crypto/rand`. Serve the page and the submit endpoint under it. Everything else returns `404` with a static body.
- [x] 5.5 Print the URL to stdout, then attempt a best-effort browser launch (`open` on darwin, `xdg-open` on linux, `rundll32` on windows). Ignore the launch's failure entirely — do not log it as an error.
- [x] 5.6 Build the page's render model from the decoded, ordered briefing and execute the embedded template. All seven presentation fields must be present, with `suggested_fix_origin` surfaced so the developer knows whose fix they are reading.
- [x] 5.7 Write the page as a single `//go:embed` HTML file: no framework, no CDN, no external asset. Batch grouping with a batch-level control plus per-member controls; a note field on every comment and on every batch.
- [x] 5.8 Implement the submit handler: decode the posted intents, expand batch-level calls to per-member entries, write the artifact, respond, then shut the server down and return so the command exits `0`.
- [x] 5.9 Handler tests via `httptest`, one per pinned scenario rather than one covering "rendered content": the `404` path carries no comment content; all seven presentation fields are present for a fixture comment; the suggested-fix origin is visible; batch members are grouped with both control levels; a note input is present per comment and per batch; batches precede non-batched comments; an empty scope renders its message and an empty-intent submit. Plus the submit round trip, asserting explicitly that no comment's `status` changed.

## 6. Reading intents back

- [x] 6.1 Add `tai triage board intents` emitting the artifact as markdown: submit timestamp, then one line per entry with ID, intent, and note, in the section-3 presentation order.
- [x] 6.2 Exit `TRIAGE_NO_INTENTS` when the scope has no artifact. Do not create one.
- [x] 6.3 End-to-end tests through `cliexec.Run` with captured buffers, per the repo's cmdtest pattern, covering emission, the `TRIAGE_NO_INTENTS` path, and the repeated-read case.

## 7. Slash command

- [x] 7.1 In `plugins/triage/assets/commands/triage.md`, add the board-offer step between §3 and §4: count survivors, offer only above five, never launch unasked. On acceptance, run §4 step 1's investigation for every survivor, assemble the briefing, and pipe it to `tai triage board -` in the background with the resolved scope flags; surface the URL, wait for exit. State plainly that this is the same investigation §4 step 1 already mandates, moved ahead of the presentation rather than replaced by it.
- [x] 7.2 Document the briefing JSON schema in `triage.md` verbatim — field list, which are required, `severity` and `suggested_fix_origin` value sets, and a worked example — so the AI assembling it never has to guess. Name the two failure codes and state that a rejected briefing lists every violation at once, so the fix is one regeneration rather than one per round trip. Document the polling fallback for a harness without exit notification: poll `tai triage board intents` and treat the `TRIAGE_NO_INTENTS` footer as not-yet, never the bare exit code.
- [x] 7.3 Add the intent-equivalence rule to §4 as its governing statement, with the per-intent consequences: accepts batched into one pass, dismissals entering §5 at their existing severity calibration, `--reason` reflecting the debate rather than the note, `unanswered` notes surfaced when the comment is presented, absent entries treated as `unanswered`, and `TRIAGE_NO_INTENTS` handled silently.
- [x] 7.4 Add the confirmation carve-out to §6: the read-back defends against misreading prose, so it does not apply to an intent-sourced split.
- [x] 7.5 Add `tai triage board` and its two sibling verbs to §9's guardrails as sanctioned calls, and restate there that the board never writes to the database.
- [x] 7.6 Re-read §§2–7 end to end and confirm no existing sentence now contradicts the board path.

## 8. PR stack

The change is estimated at ~1500 countable diff lines, over the 800-line cap in `docs/PR_WORKFLOW.md`. Invoke the `stacked-pr` skill before opening anything.

Each slice carries the `test-cases.md` entries for the behaviour it implements. `docs/PR_WORKFLOW.md` names a BDD entry separated from the test that claims its TC-ID as one of three seams that are never valid, so no slice may contain a test whose ID has no Given/When/Then in the same diff.

- [x] 8.1 Slice 1 — error codes, briefing types + validator + ordering, the intents artifact package (sections 2, 3, 4), carrying the error-code, briefing-validation and artifact TC-BRD entries.
- [x] 8.2 Slice 2 — board server, embedded page, submit handler (section 5), carrying the bind, path-prefix, browser-launch, submit and rendering TC-BRD entries.
- [x] 8.3 Slice 3 — `board intents`, slash-command contract (sections 6, 7), carrying the intents-emission TC-BRD entries and the manual-coverage declaration.
- [x] 8.4 Merge vehicle branched from the tip of slice 3. It carries no entries of its own — every TC-BRD entry has already landed with its tests.

## 9. Verification

- [x] 9.1 `go build ./... && go test ./... && go vet ./... && gofmt -l .` clean; `go test -race ./...` clean.
- [x] 9.2 `golangci-lint run` at zero.
- [x] 9.3 Manual pass: run a real triage against a PR with more than five surviving comments, accept the board offer, bulk-accept the majority, dismiss one `critical` with a note, leave several unanswered with and without notes, and confirm the conversation debates the critical, batches the accepts, and walks the unanswered ones in order.
- [ ] 9.4 Manual pass on a headless host: confirm the URL prints, the server keeps running, and the board works over a forwarded port. NOT DONE at archive time — no headless host was available. The behaviour is covered by TC-BRD-011's declaration that a failed browser launch is not fatal, which is itself manual; the forwarded-port path has never been exercised.
- [x] 9.5 Archive this change to `openspec/changes/archive/<merge-date>-add-triage-board/` in the same commit as the implementation.
