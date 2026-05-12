# Implementation Tasks

> **Prerequisite (out of pipeline):** `add-tai-foundation`, `add-storage-schema`, and `add-import-command` are implemented. Rows are populating; this layer reads and mutates them.
>
> Add BDD cases under the `TRG` category (new — add to `test-cases.md` ToC) before each TDD slice.

## 1. Scope resolution

- [ ] 1.1 Implement `internal/triage/scope.Resolve(ctx) (Scope, error)` returning the resolved PR or branch row in the current repo
- [ ] 1.2 Read the current git branch via `git rev-parse --abbrev-ref HEAD`; tolerate detached HEAD (treat as "no current branch")
- [ ] 1.3 Implement precedence: `--pr` → `--branch` → auto-detect
- [ ] 1.4 Mutex check: `--pr` and `--branch` together → `TRIAGE_INVALID_FLAGS`
- [ ] 1.5 TC-TRG-001..007: each branch of the precedence (PR flag, branch flag, auto-detect PR, auto-detect branch, both match, neither matches, mutex)

## 2. Per-target sequential ID translation

- [ ] 2.1 Implement `internal/triage/idx.LookupByPos(scope, pos int) (commentID int64, error)` using `ROW_NUMBER() OVER (...)` to translate user-facing positions to internal DB IDs
- [ ] 2.2 Implement reverse: `PositionFor(commentID) int` for emitting positions in show/list output
- [ ] 2.3 TC-TRG-010..012: positions start at 1, are scoped per target, shift after delete

## 3. `tai list`

- [ ] 3.1 Wire `tai list` subcommand
- [ ] 3.2 Query comments in scope, compute positions, emit the table per spec
- [ ] 3.3 Severity abbreviation map (`critical→crit`, `major→maj`, `minor→min`, `nitpick→nit`)
- [ ] 3.4 Title truncation: detect terminal width via `golang.org/x/term` (or stdlib); fall back to 80 columns
- [ ] 3.5 Empty-scope rendering: header + `(no comments)`
- [ ] 3.6 Implement `--status <state>` flag (repeatable); multiple values combine via OR; unknown value → `TRIAGE_INVALID_FLAGS`
- [ ] 3.7 TC-TRG-020..026: list with comments, list empty, severity abbreviation, title truncation, single-status filter, multi-status filter, unknown status

## 4. `tai show`

- [ ] 4.1 Wire `tai show <id>` and `tai show --all` subcommands
- [ ] 4.2 Implement the markdown renderer: header line, meta lines, conditional Batch meta, all five enrichment sections, conditional Resolution/Dismissed-because sections
- [ ] 4.3 Implement `--all` separator handling (`\n\n---\n\n` between blocks)
- [ ] 4.4 Implement `--status <state>` flag (repeatable) on `tai show --all` only; reject on single-comment form
- [ ] 4.5 TC-TRG-030..038: pending comment (no resolution/dismiss sections), accepted with resolution, dismissed, batch meta line, --all with two comments, --all empty (zero-byte stdout), section order/names, --all --status filters output, --status rejected on single-comment form

## 5. `tai accept`

- [ ] 5.1 Wire `tai accept <id>` and `tai accept --batch <key>` subcommands
- [ ] 5.2 State transition: status → `accepted`, set `resolution` from `--resolution` if provided, clear `dismissed_by`/`dismiss_reason`
- [ ] 5.3 Idempotent on already-accepted (no-op when no `--resolution`, update resolution when provided)
- [ ] 5.4 Mutex check: `<id>` and `--batch` together → `TRIAGE_INVALID_FLAGS`
- [ ] 5.5 Batch mode: apply transition to every member, recompute batch status
- [ ] 5.6 TC-TRG-040..046: accept pending, accept dismissed (reversal), idempotent, accept by batch, mutex flags, accept non-existent ID, clears dismissal fields

## 6. `tai dismiss`

- [ ] 6.1 Wire `tai dismiss <id>` subcommand with REQUIRED `--reason` flag
- [ ] 6.2 Resolve `--by` default: `git config --get user.name` → `$USER` → `"unknown"`
- [ ] 6.3 State transition: status → `dismissed`, set `dismiss_reason`/`dismissed_by`, clear `resolution`
- [ ] 6.4 Batch support: `tai dismiss --batch <key> --reason <text>`
- [ ] 6.5 TC-TRG-050..055: dismiss with reason, missing --reason errors, dismiss reverses an accepted state, --by override, dismiss by batch, dismiss non-existent ID

## 7. `tai complete`

- [ ] 7.1 Wire `tai complete <id>` and `tai complete --batch <key>` subcommands
- [ ] 7.2 State transition: status → `completed`, set `resolution` from `--resolution` if provided, clear `dismissed_by`/`dismiss_reason`
- [ ] 7.3 Same idempotency / batch / mutex rules as `tai accept`
- [ ] 7.4 TC-TRG-060..062: complete pending, complete by batch, complete state reversal from dismissed

## 8. Batch status recomputation

- [ ] 8.1 Implement `internal/triage/batch.Recompute(tx, batchID)`: read all member statuses, compute the new batch status (`pending` | terminal-uniform | `mixed`), UPDATE the batch row
- [ ] 8.2 Hook recompute into accept/dismiss/complete after every per-member or batch transition (inside the same transaction)
- [ ] 8.3 TC-TRG-070..072: all members terminal-uniform → batch matches; split → mixed; pending+terminal → mixed

## 9. `tai status`

- [ ] 9.1 Wire `tai status` subcommand
- [ ] 9.2 Emit the summary block per spec: repo line, scope line, counts block, optional batches block
- [ ] 9.3 Branch vs PR target-label rendering
- [ ] 9.4 Omit Batches block when zero batches; omit per-status lines when zero (except `Total`)
- [ ] 9.5 TC-TRG-080..082: PR status with batches, branch status without batches, empty scope

## 10. `tai forget`

- [ ] 10.1 Wire `tai forget` subcommand with mutex selectors: `--comment`, `--batch`, `--pr`, `--branch`, `--repo`
- [ ] 10.2 Each selector resolves to a delete plan (the row to delete, plus a count of cascaded rows)
- [ ] 10.3 Plan summary printer: enumerate human-readable bullets and the consent prompt
- [ ] 10.4 Consent gate: TTY prompt; `--yes` flag; `TAI_ACCEPT_DESTRUCTIVE` env var (truthy semantics — reuse `isTruthyEnv`)
- [ ] 10.5 Non-interactive without consent → exit `1` with `TRIAGE_CONFIRMATION_REQUIRED` (loud failure, NOT silent skip)
- [ ] 10.6 Delete inside one transaction; let CASCADE rules from `add-storage-schema` handle children
- [ ] 10.7 `--repo` mode bypasses scope resolution (runs anywhere)
- [ ] 10.8 Implement `--status <state>` modifier (repeatable) on `--pr`, `--branch`, `--repo`, `--batch`; reject on `--comment`
- [ ] 10.9 When `--status` is present: build the delete plan over filtered comments only; preserve parent rows; for `--batch + --status`, recompute the batch status after the delete (reuse the recompute helper from §8)
- [ ] 10.10 Summary block names the count of matching comments (not the parent row) when `--status` is in play
- [ ] 10.11 TC-TRG-090..0aa: each selector + consent path (interactive yes, interactive no, --yes, env var, non-interactive without consent, zero selectors, two selectors, --repo outside git, cascade count accurate, comment ID position shift after delete, --status on PR, --status on repo, --status on batch with recompute, --status rejected on --comment, multi-value --status

## 11. Error codes

- [ ] 11.1 Extend `internal/errcode` taxonomy with the five new codes (`TRIAGE_NO_SCOPE`, `TRIAGE_AMBIGUOUS_SCOPE`, `TRIAGE_NOT_FOUND`, `TRIAGE_INVALID_FLAGS`, `TRIAGE_CONFIRMATION_REQUIRED`)
- [ ] 11.2 Verify the in-code table matches `specs/cli-framework/spec.md` after the delta
- [ ] 11.3 TC-TRG-100..104: each new code produces the standard footer

## 12. Documentation and validation

- [ ] 12.1 Add `TRG` to `test-cases.md` ToC
- [ ] 12.2 Author package doc in `internal/triage` pointing at `specs/triage/spec.md`
- [ ] 12.3 `go test ./... && go vet ./... && gofmt -l .` clean; `go test -race ./...` clean
- [ ] 12.4 `openspec validate add-triage-state` reports no errors
