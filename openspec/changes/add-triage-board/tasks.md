## 1. BDD cases — add TC-IDs before any code

- [ ] 1.1 In `plugins/triage/test-cases.md`, add a new `BRD` category to the ToC table with scope "the triage board: loopback server, rendered content, intents artifact, `board intents` / `board status`".
- [ ] 1.2 Pin `TC-BRD-*` IDs for the server: loopback bind on an ephemeral port with the URL on stdout; request without the path prefix returns `404` carrying no comment content; browser-launch failure keeps the server running; submit writes the artifact and exits `0` with no database mutation; bind failure exits `TRIAGE_BOARD_UNAVAILABLE`.
- [ ] 1.3 Pin `TC-BRD-*` IDs for rendered content: only `pending` comments appear; every `tai show` field is present; batch members grouped under key and title with both a batch-level and per-member control; batches precede non-batched comments; empty scope renders its message.
- [ ] 1.4 Pin `TC-BRD-*` IDs for the artifact: all three intents recorded; note on an `unanswered`; batch call expands to per-member entries with no batch-level entry; resubmission replaces; scope-keyed paths do not cross-read.
- [ ] 1.5 Pin `TC-BRD-*` IDs for `board intents`: markdown emission with timestamp and one line per entry in presentation order; `TRIAGE_NO_INTENTS` when absent; repeated reads are identical and non-destructive.
- [ ] 1.6 Pin `TC-BRD-*` IDs for `board status`: exits `0` with "none submitted" before submit and with the timestamp after.
- [ ] 1.7 Pin `TC-BRD-*` IDs for the two error codes rendering through the foundation template with the correct `[exit N: CODE]` footer.
- [ ] 1.8 Add a note in the same file, alongside the existing note about the slash commands, recording that the board's browser page has no automated coverage: handler responses, the artifact, and `board intents` output are covered; in-page interaction is exercised manually.
- [ ] 1.9 Cross-check every new TC-ID drives at the boundary the user observes, per CLAUDE.md's north-star rule. A TC about stdout gets a test capturing stdout; a TC about the artifact gets a test inspecting the written file; a TC about an HTTP response gets a handler test.

## 2. Error codes

- [ ] 2.1 In `pkg/errcode/errcode.go`, append `TriageBoardUnavailable` (`TRIAGE_BOARD_UNAVAILABLE`) and `TriageNoIntents` (`TRIAGE_NO_INTENTS`) to the triage-layer block. Append-only — do not reorder or repurpose existing codes.
- [ ] 2.2 Map exit codes: `TriageBoardUnavailable` → `1`; `TriageNoIntents` → `2`, alongside `TriageNotFound`.
- [ ] 2.3 Add the two codes to `pkg/test-cases.md`'s error-code taxonomy section if it enumerates codes individually.

## 3. Storage read surface

- [ ] 3.1 Add a query to `plugins/triage/internal/storage/` returning every `pending` comment in a scope with all display fields plus its batch key and batch title, ordered batches-first by highest-severity member then batch key, then non-batched by severity then file then lines. This is the single source both the board and the ordering requirement read from.
- [ ] 3.2 Unit-test the ordering directly, including the tie-breaks, against a fixture with two batches and several non-batched comments.

## 4. Intents artifact

- [ ] 4.1 Create `plugins/triage/internal/board/` with the artifact's Go types: repo, scope, `submitted_at` (RFC 3339), and entries of `{id, intent, note}` where intent is `accept` | `dismiss` | `unanswered`.
- [ ] 4.2 Implement the scope-keyed path under `<TAI_DATA_DIR>/plugins/triage/state/intents/` as `<owner>--<name>--pr-<number>.json` or `<owner>--<name>--branch-<slug>.json`. Slug branch names so a `/` in a branch cannot escape the directory.
- [ ] 4.3 Implement read and write. Write replaces the file in full. Read returns a typed "absent" result rather than an error so callers distinguish absence from an unreadable file.
- [ ] 4.4 Test round-tripping, replacement on resubmit, path derivation for both scope kinds, and that a branch name containing `/` produces a path inside the intents directory.

## 5. Board server and page

- [ ] 5.1 Add `plugins/triage/internal/cmd/board.go` wiring `tai triage board` with the shared `--pr` / `--branch` scope flags, and register it in `plugins/triage/internal/cmd/root.go`.
- [ ] 5.2 Bind `127.0.0.1:0`. On bind failure return `TRIAGE_BOARD_UNAVAILABLE`. Read the assigned port back from the listener and build the URL from it — never assume a port.
- [ ] 5.3 Generate a 32-hex path prefix per invocation from `crypto/rand`. Serve the page and the submit endpoint under it. Everything else returns `404` with a static body.
- [ ] 5.4 Print the URL to stdout, then attempt a best-effort browser launch (`open` on darwin, `xdg-open` on linux, `rundll32` on windows). Ignore the launch's failure entirely — do not log it as an error.
- [ ] 5.5 Build the page's render model from the section-3 query and execute the embedded template. Every field `tai show` renders must be present.
- [ ] 5.6 Write the page as a single `//go:embed` HTML file: no framework, no CDN, no external asset. Batch grouping with a batch-level control plus per-member controls; a note field on every comment and on every batch.
- [ ] 5.7 Implement the submit handler: decode the posted intents, expand batch-level calls to per-member entries, write the artifact, respond, then shut the server down and return so the command exits `0`.
- [ ] 5.8 Handler tests via `httptest` for the `404` path, the rendered content, and the submit round trip. Assert explicitly in the submit test that no comment's `status` changed.

## 6. Reading intents back

- [ ] 6.1 Add `tai triage board intents` emitting the artifact as markdown: submit timestamp, then one line per entry with ID, intent, and note, in the section-3 presentation order.
- [ ] 6.2 Exit `TRIAGE_NO_INTENTS` when the scope has no artifact. Do not create one.
- [ ] 6.3 Add `tai triage board status` reporting presence and timestamp, exiting `0` either way.
- [ ] 6.4 End-to-end tests through `cliexec.Run` with captured buffers, per the repo's cmdtest pattern, covering both verbs including the repeated-read case.

## 7. Slash command

- [ ] 7.1 In `plugins/triage/assets/commands/triage.md`, add the board-offer step between §3 and §4: count survivors, offer only above five, never launch unasked, launch in the background with the resolved scope flags, surface the URL, wait for exit. Document the `tai triage board status` polling fallback for a harness without exit notification.
- [ ] 7.2 Add the intent-equivalence rule to §4 as its governing statement, with the per-intent consequences: accepts batched into one pass, dismissals entering §5 at their existing severity calibration, `--reason` reflecting the debate rather than the note, `unanswered` notes surfaced when the comment is presented, absent entries treated as `unanswered`, and `TRIAGE_NO_INTENTS` handled silently.
- [ ] 7.3 Add the confirmation carve-out to §6: the read-back defends against misreading prose, so it does not apply to an intent-sourced split.
- [ ] 7.4 Add `tai triage board` and its two sibling verbs to §9's guardrails as sanctioned calls, and restate there that the board never writes to the database.
- [ ] 7.5 Re-read §§2–7 end to end and confirm no existing sentence now contradicts the board path.

## 8. PR stack

The change is estimated at ~1500 countable diff lines, over the 800-line cap in `docs/PR_WORKFLOW.md`. Invoke the `stacked-pr` skill before opening anything.

- [ ] 8.1 Slice 1 — error codes, storage query, intents artifact package (sections 2, 3, 4).
- [ ] 8.2 Slice 2 — board server, embedded page, submit handler (section 5).
- [ ] 8.3 Slice 3 — `board intents`, `board status`, slash-command contract (sections 6, 7).
- [ ] 8.4 Merge vehicle carrying all three plus the `test-cases.md` additions from section 1.

## 9. Verification

- [ ] 9.1 `go build ./... && go test ./... && go vet ./... && gofmt -l .` clean; `go test -race ./...` clean.
- [ ] 9.2 `golangci-lint run` at zero.
- [ ] 9.3 Manual pass: run a real triage against a PR with more than five surviving comments, accept the board offer, bulk-accept the majority, dismiss one `critical` with a note, leave several unanswered with and without notes, and confirm the conversation debates the critical, batches the accepts, and walks the unanswered ones in order.
- [ ] 9.4 Manual pass on a headless host: confirm the URL prints, the server keeps running, and the board works over a forwarded port.
- [ ] 9.5 Archive this change to `openspec/changes/archive/<merge-date>-add-triage-board/` in the same commit as the implementation.
