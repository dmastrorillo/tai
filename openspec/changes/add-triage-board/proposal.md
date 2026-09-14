## Why

A triage session on a well-reviewed PR routinely carries 30–40 comments, and the overwhelming majority of them are accepted without argument. Today `/tai-triage:triage` walks every one of those through the same one-at-a-time conversational loop: present the comment, ask "accept, dismiss, or complete?", wait, persist, emit a progress line. For the comments that warrant a debate that loop is exactly right — it is the product. For the thirty-seven that a developer reads and immediately accepts, it is thirty-seven round trips to say "yes" thirty-seven times.

The bottleneck is not the developer's judgement, which is fast. It is that the only surface for expressing that judgement is a serialised conversation, and a conversation can only present one comment at a time.

This change adds a **board**: a browser surface, served on loopback by the triage plugin, that shows every pending comment in a scope at once — carrying the same seven investigated fields the loop presents one at a time today — and lets the developer accept, dismiss, or leave each one alone in a single pass, attaching a free-text note to any of them. The board records those calls as **intents** and writes them to a file. It never touches the database.

The triage loop then reads the intents and proceeds exactly as it does today, treating each intent as though the developer had typed that answer at that comment's decision prompt. A bulk-dismissed `critical` still gets the full dismissal-debate contract. Comments the developer left alone are `unanswered` and are walked one at a time, unchanged.

The goal is not to triage faster or with less rigour. It is to spend the conversation's serialised bandwidth on the comments that actually need it.

## What Changes

- **New capability `triage-board`**, adding two verbs to the triage plugin:
  - `tai triage board - [--pr N | --branch NAME]` — reads the AI's briefing as JSON on stdin, validates it, binds an HTTP listener on `127.0.0.1:0`, serves the board under an unguessable random path, opens the developer's browser (best-effort), and blocks. On submit it writes the intents artifact and exits `0`. It opens no database.
  - `tai triage board intents [--pr N | --branch NAME]` — reads the intents artifact for a scope and emits it as markdown for the AI to consume. Exits `TRIAGE_NO_INTENTS` while no artifact exists, which is also the not-yet signal an agent harness polls when it cannot be notified of the board process exiting.
- **New on-disk artifact**: the intents file, at `<TAI_DATA_DIR>/plugins/triage/state/intents/<owner>--<name>--<scope>.json`. Written only by `tai triage board`, read only by `tai triage board intents`.
- **`triage-command` gains a board-offer step** between phase 1 (investigation) and phase 2 (the loop). When more than five comments remain pending after investigation, the AI offers the board. It never launches the board without the developer asking for it.
- **`triage-command`'s phase-2 loop is seeded from intents.** The governing rule: an intent is exactly equivalent to the developer having typed that answer at that comment's decision prompt. Every existing obligation — the dismissal-debate contract calibrated by severity, the batch-first severity ordering, the recap — applies unchanged.
- **`triage-command`'s batch-override confirmation is scoped to conversational input.** The read-back-and-wait step exists to defend against the AI misparsing prose such as "accept B1 except B1.4". A board-sourced split carries no parse, so the confirmation does not apply to it. The batch still lands `mixed`.
- **Four new error codes**: `TRIAGE_BOARD_INVALID_JSON`, `TRIAGE_BOARD_SCHEMA_INVALID`, `TRIAGE_BOARD_UNAVAILABLE`, `TRIAGE_NO_INTENTS`.

## Non-Goals

- **The board does not write to the database.** Comment status is changed only by `tai triage accept` / `tai triage dismiss` / `tai triage complete`, invoked by the AI after the triage conversation. This is the property that makes the change additive; it is a constraint, not an implementation detail that may be relaxed later for convenience.
- **No change to the storage schema.** No migration, no new status value, no change to the comment state machine. Intents are not a comment state, and `cause` lives in the briefing rather than becoming a column.
- **No change to the `triage` capability's seven existing verbs.**
- **No remote access.** The listener binds loopback only. There is no multi-user mode, no shared board, no hosted surface.
- **No new dependency.** The server is `net/http`; the page is a single file embedded with `embed`. No TUI library, no frontend framework, no CDN.
- **The board does not investigate.** The AI does, exactly as the loop's presentation contract already requires, and hands the board the result. The board renders what it is given and derives nothing.
- **The presentation is the only thing that changes.** Scope resolution, the phase-1 already-fixed check, the per-comment investigation, the dismissal-debate contract, batch overrides and the recap are all untouched. The loop stops typing the investigation into the conversation one comment at a time and shows it all at once instead.

## Capabilities

### New Capabilities

- `triage-board`: the board's two verbs, the briefing JSON schema and its validation contract, the loopback server's lifecycle and security posture, what the board renders, the intents artifact's format and location, and the four new error codes.

### Modified Capabilities

- `triage-command`: adds the board-offer step and the intent-equivalence rule to the four-phase loop; scopes the batch-override confirmation requirement to conversational input.

## Impact

- **Code (all under `plugins/triage/`)**: new `internal/cmd/board.go` (server lifecycle, submit handler, browser launch), new `internal/board/` package (intents artifact read/write, the embedded page, the render model), a new pending-comments-with-batches query in `internal/storage/`, and wiring in `internal/cmd/root.go`.
- **`pkg/errcode`**: append `TriageBoardUnavailable` and `TriageNoIntents` (append-only registry).
- **On-disk state**: new `<TAI_DATA_DIR>/plugins/triage/state/intents/` directory. Not read by any other component.
- **Assets**: `plugins/triage/assets/commands/triage.md` gains the board-offer step, the briefing schema, the intent-equivalence rule, and the batch-confirmation carve-out.
- **Spec correction carried by the `Phase 2` MODIFIED block**: the long-lived requirement still describes step 1 as surfacing `tai triage show` verbatim and step 2 as asking for three verbs. The shipped command has presented an investigation under seven fields and asked for two verbs since TC-AST-003, and a MODIFIED block replaces the requirement wholesale, so restating the old text would re-assert something the plugin now forbids. The block states the shipped behaviour. The same staleness in that capability's other requirements is corrected alongside it.
- **Tests**: new `TC-BRD-*` category in `plugins/triage/test-cases.md`.
- **Docs**: `CONTEXT.md` carries the `Board`, `Intent`, `Decision`, and `Bulk pass` glossary entries. Its opening sentence is also corrected there: it pointed design rationale at `docs/adr/`, a directory this repo does not have, and now points at `openspec/` — the change proposal for the reasoning, the capability spec for the settled contract.
- **Not breaking.** Every existing invocation of `/tai-triage:triage` and every existing `tai triage` verb behaves as before. A developer who never accepts the board's offer sees no change at all.
- **PR size**: the change is estimated at ~1500 countable diff lines, over the 800-line cap in `docs/PR_WORKFLOW.md`. It ships as a stack of three slices plus a merge vehicle; see `tasks.md` section 8.
