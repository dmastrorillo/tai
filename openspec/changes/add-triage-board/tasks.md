## 1. BDD cases — add TC-IDs before any code

Section 8 assigns each group below to the slice that implements it; the entry and the tests naming it always land in the same diff.

- [ ] 1.1 In `plugins/triage/test-cases.md`, add a new `BRD` category to the ToC table with scope "the triage board: loopback server, rendered content, intents artifact, `board intents`".
- [ ] 1.2 Pin `TC-BRD-*` IDs for the server: loopback bind on an ephemeral port with the URL on stdout; request without the path prefix returns `404` carrying no comment content; browser-launch failure keeps the server running; submit writes the artifact and exits `0` with no database mutation; bind failure exits `TRIAGE_BOARD_UNAVAILABLE`.
- [ ] 1.3 Pin `TC-BRD-*` IDs for rendered content: only `pending` comments appear; every field `tai triage show` renders is present; batch members grouped under key and title with both a batch-level and per-member control; batches precede non-batched comments; a note input is present for every comment and every batch; empty scope renders its message and offers submit of an empty intent set. Each of these asserts against the served HTML, so each is reachable from a handler test.
- [ ] 1.4 Pin `TC-BRD-*` IDs for the artifact: all three intents recorded; note on an `unanswered`; batch call expands to per-member entries with no batch-level entry; resubmission replaces; scope-keyed paths do not cross-read.
- [ ] 1.5 Pin `TC-BRD-*` IDs for `board intents`: markdown emission with timestamp and one line per entry in presentation order; `TRIAGE_NO_INTENTS` when absent; repeated reads are identical and non-destructive.
- [ ] 1.6 Pin `TC-BRD-*` IDs for the two error codes rendering through the foundation template with the correct `[exit N: CODE]` footer.
- [ ] 1.7 Add a note in the same file recording exactly what the board leaves to manual verification: the served HTML, the intents artifact and `tai triage board intents` output all carry TC-BRD IDs and Go tests; what a click does in the page once that HTML is loaded does not. State the boundary explicitly rather than by analogy to the slash commands, whose note covers conversational contracts that carry no TC-IDs at all — the board's exemption is the narrower one.
- [ ] 1.8 Cross-check every new TC-ID drives at the boundary the user observes, per CLAUDE.md's north-star rule. A TC about stdout gets a test capturing stdout; a TC about the artifact gets a test inspecting the written file; a TC about an HTTP response gets a handler test.

## 2. Error codes

- [ ] 2.1 In `pkg/errcode/errcode.go`, append `TriageBoardUnavailable` (`TRIAGE_BOARD_UNAVAILABLE`) and `TriageNoIntents` (`TRIAGE_NO_INTENTS`) to the triage-layer block. Append-only — do not reorder or repurpose existing codes.
- [ ] 2.2 Map exit codes: `TriageBoardUnavailable` → `exitcode.Data` (`3`), alongside `DataDirUnwritable` and the other environment-blocked codes; `TriageNoIntents` → `exitcode.Precondition` (`2`), alongside `TriageNotFound`.
- [ ] 2.3 Add the two codes to `pkg/test-cases.md`'s error-code taxonomy section if it enumerates codes individually.

## 3. Presentation ordering

- [ ] 3.1 Add an exported comparator over `[]triage.Comment` in `plugins/triage/internal/triage/` implementing the presentation order: batches first by highest-severity member then batch key ascending, then non-batched by severity then file then lines. It is the one place that order is encoded, shared by the board and by the slash command's phase-2 loop. Add no SQL: `listSQL` in that package already projects every field the board renders, plus `position`, `batch_key` and the batch title, so the board calls `ListComments(ctx, db, scope, []string{"pending"})` and sorts the result.
- [ ] 3.2 Unit-test the comparator directly, including both tie-breaks, against a fixture with two batches and several non-batched comments.

## 4. Intents artifact

- [ ] 4.1 Create `plugins/triage/internal/board/` with the artifact's Go types: repo, scope, `submitted_at` (RFC 3339), and entries of `{id, intent, note}` where intent is `accept` | `dismiss` | `unanswered`.
- [ ] 4.2 Implement the scope-keyed path under `<TAI_DATA_DIR>/plugins/triage/state/intents/` as `<owner>--<name>--pr-<number>.json` or `<owner>--<name>--branch-<slug>.json`. Slug branch names so a `/` in a branch cannot escape the directory.
- [ ] 4.3 Implement read and write. Write replaces the file in full. Read returns a typed "absent" result rather than an error so callers distinguish absence from an unreadable file.
- [ ] 4.4 Test round-tripping, replacement on resubmit, path derivation for both scope kinds, and that a branch name containing `/` produces a path inside the intents directory.

## 5. Board server and page

- [ ] 5.1 Add `plugins/triage/internal/cmd/board.go` wiring `tai triage board` with the shared `--pr` / `--branch` scope flags, and register it in `plugins/triage/internal/cmd/root.go`.
- [ ] 5.2 Bind `127.0.0.1:0` through a package-level `var listen = net.Listen`. On bind failure return `TRIAGE_BOARD_UNAVAILABLE`. Read the assigned port back from the listener and build the URL from it — never assume a port.
- [ ] 5.3 Add `ListenFailureForTesting(t testing.TB, err error)` swapping `listen` for one that returns `err`, restoring it via `t.Cleanup`. Follows the established seam pattern — `config.AllowFileURLsForTesting`, `plugins.RegisterForTesting`, `sync.AutoInstallForTesting` — where the `testing.TB` parameter makes production use a code-review red flag. A kernel-assigned port has no deterministic way to fail, so this seam is the only route to the bind-failure case, and that case is what pins `TRIAGE_BOARD_UNAVAILABLE`'s exit code against an append-only registry. Its test asserts the error code, the exit code, and that no intents artifact is written.
- [ ] 5.4 Generate a 32-hex path prefix per invocation from `crypto/rand`. Serve the page and the submit endpoint under it. Everything else returns `404` with a static body.
- [ ] 5.5 Print the URL to stdout, then attempt a best-effort browser launch (`open` on darwin, `xdg-open` on linux, `rundll32` on windows). Ignore the launch's failure entirely — do not log it as an error.
- [ ] 5.6 Build the page's render model from `ListComments` filtered to `pending` and sorted with the section-3 comparator, then execute the embedded template. Every field `tai triage show` renders must be present.
- [ ] 5.7 Write the page as a single `//go:embed` HTML file: no framework, no CDN, no external asset. Batch grouping with a batch-level control plus per-member controls; a note field on every comment and on every batch.
- [ ] 5.8 Implement the submit handler: decode the posted intents, expand batch-level calls to per-member entries, write the artifact, respond, then shut the server down and return so the command exits `0`.
- [ ] 5.9 Handler tests via `httptest`, one per pinned scenario rather than one covering "rendered content": the `404` path carries no comment content; only `pending` comments appear; every field `tai triage show` renders is present for a fixture comment; batch members are grouped with both control levels; a note input is present per comment and per batch; batches precede non-batched comments; an empty scope renders its message and an empty-intent submit. Plus the submit round trip, asserting explicitly that no comment's `status` changed.

## 6. Reading intents back

- [ ] 6.1 Add `tai triage board intents` emitting the artifact as markdown: submit timestamp, then one line per entry with ID, intent, and note, in the section-3 presentation order.
- [ ] 6.2 Exit `TRIAGE_NO_INTENTS` when the scope has no artifact. Do not create one.
- [ ] 6.3 End-to-end tests through `cliexec.Run` with captured buffers, per the repo's cmdtest pattern, covering emission, the `TRIAGE_NO_INTENTS` path, and the repeated-read case.

## 7. Slash command

- [ ] 7.1 In `plugins/triage/assets/commands/triage.md`, add the board-offer step between §3 and §4: count survivors, offer only above five, never launch unasked, launch in the background with the resolved scope flags, surface the URL, wait for exit. Document the polling fallback for a harness without exit notification: poll `tai triage board intents` and treat the `TRIAGE_NO_INTENTS` footer as not-yet, never the bare exit code.
- [ ] 7.2 Add the intent-equivalence rule to §4 as its governing statement, with the per-intent consequences: accepts batched into one pass, dismissals entering §5 at their existing severity calibration, `--reason` reflecting the debate rather than the note, `unanswered` notes surfaced when the comment is presented, absent entries treated as `unanswered`, and `TRIAGE_NO_INTENTS` handled silently.
- [ ] 7.3 Add the confirmation carve-out to §6: the read-back defends against misreading prose, so it does not apply to an intent-sourced split.
- [ ] 7.4 Add `tai triage board` and its two sibling verbs to §9's guardrails as sanctioned calls, and restate there that the board never writes to the database.
- [ ] 7.5 Re-read §§2–7 end to end and confirm no existing sentence now contradicts the board path.

## 8. PR stack

The change is estimated at ~1500 countable diff lines, over the 800-line cap in `docs/PR_WORKFLOW.md`. Invoke the `stacked-pr` skill before opening anything.

Each slice carries the `test-cases.md` entries for the behaviour it implements. `docs/PR_WORKFLOW.md` names a BDD entry separated from the test that claims its TC-ID as one of three seams that are never valid, so no slice may contain a test whose ID has no Given/When/Then in the same diff.

- [ ] 8.1 Slice 1 — error codes, the ordering comparator, the intents artifact package (sections 2, 3, 4), carrying the error-code and artifact TC-BRD entries.
- [ ] 8.2 Slice 2 — board server, embedded page, submit handler (section 5), carrying the bind, path-prefix, browser-launch, submit and rendering TC-BRD entries.
- [ ] 8.3 Slice 3 — `board intents`, slash-command contract (sections 6, 7), carrying the intents-emission TC-BRD entries and the manual-coverage declaration.
- [ ] 8.4 Merge vehicle branched from the tip of slice 3. It carries no entries of its own — every TC-BRD entry has already landed with its tests.

## 9. Verification

- [ ] 9.1 `go build ./... && go test ./... && go vet ./... && gofmt -l .` clean; `go test -race ./...` clean.
- [ ] 9.2 `golangci-lint run` at zero.
- [ ] 9.3 Manual pass: run a real triage against a PR with more than five surviving comments, accept the board offer, bulk-accept the majority, dismiss one `critical` with a note, leave several unanswered with and without notes, and confirm the conversation debates the critical, batches the accepts, and walks the unanswered ones in order.
- [ ] 9.4 Manual pass on a headless host: confirm the URL prints, the server keeps running, and the board works over a forwarded port.
- [ ] 9.5 Archive this change to `openspec/changes/archive/<merge-date>-add-triage-board/` in the same commit as the implementation.
