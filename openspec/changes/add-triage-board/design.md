## Context

`/tai-triage:triage` is a conversational loop. The `triage` capability provides seven impartial CLI verbs; the `triage-command` capability provides the opinionated slash command that drives them, and everything subjective — presentation order, the dismissal debate, cognitive-bias checks — lives in the slash command's body rather than in Go.

That design is deliberate and this change does not disturb it. What it does disturb is the assumption that a conversation is the only input surface. A conversation is serial: it presents one comment, waits, persists, and moves on. That is the right shape for a decision that needs discussion and the wrong shape for a decision that is already made before the comment finishes rendering.

Observed usage: a 40-comment triage session in which 37 comments were accepted and 3 were dismissed. The three dismissals were worth the debate. The 37 accepts cost 37 round trips to express judgement that was formed in seconds.

The problem is a mismatch between the bandwidth of the input surface and the bandwidth of the decision, not a problem with the rigour of the process. Any solution that reduces rigour to gain speed solves the wrong problem.

## Goals / Non-Goals

**Goals:**

1. Let a developer read every pending comment in a scope — with the same content `tai triage show` renders — and record accept / dismiss / leave-alone calls on all of them in one pass.
2. Let any of those calls carry a free-text note: a refinement to the suggested fix, the reasoning behind a dismissal, or a request for explanation on one left alone.
3. Feed those calls into the existing triage loop such that every existing obligation still fires. A bulk dismissal of a `critical` gets the same debate it would get if typed in chat.
4. Leave the default `/tai-triage:triage` path byte-identical for anyone who does not use the board.
5. Add no dependency, no schema migration, and no new comment state.

**Non-Goals:**

1. Speeding up dismissals. Dismissal rigour is the product; the board's saving comes from accepts and from not spending conversational bandwidth on them.
2. Replacing the one-at-a-time loop. The board narrows what the loop covers; it never substitutes for it.
3. A general-purpose review UI. The board renders `pending` comments in one scope and captures three intents. It does not browse repos, PRs, history, or completed work.

## Decisions

### The board never writes to the database

The board reads comments and writes intents to a file. It does not call `tai triage accept` / `tai triage dismiss` / `tai triage complete`, and it does not open the database for writing.

This is the load-bearing decision of the whole change, and everything cheap about the change follows from it:

- No schema migration. No new `status` value, no `proposed_dismissal` state, no `decided_via` column.
- No dilution of what `dismissed` means. In the database, `dismissed` continues to mean "went through the dismissal-debate contract and came out the other side", because a bulk dismissal never writes `dismissed` — it writes an intent, and the debate still happens before anything is persisted.
- The `triage-command` guardrail that the CLI verbs are the only persistence seam survives untouched. The board is not a second writer; it is a second *reader* plus a scratch file.

The alternative — having the board persist accepts directly and leave only dismissals for the AI — was rejected. It would save one round trip per accept at the cost of two independent writers to the same state, a new class of partial-completion states to reason about, and a migration. The equivalence rule below already collapses accepts into a single batched pass, which captures nearly all of the saving with none of the cost.

Corollary worth stating because it will be tempting later: if a future change wants the board to write directly "just for accepts", it is no longer this change's architecture and needs its own proposal arguing why the second writer is worth it.

### An intent is exactly equivalent to a typed answer

The rule the slash command is built around:

> An intent is exactly equivalent to the developer having typed that answer at that comment's decision prompt in phase 2.

Stated this way, the entire behavioural surface of the change is one sentence, and no obligation needs restating per-intent. `critical` dismissals get the concrete-scenario challenge. `minor` and `nitpick` dismissals are not debated and the note becomes the reason verbatim. Accepts persist without argument, with any note captured as `--resolution`. A dismissal with no note is "I don't want to fix that" with no reasoning, which the dismissal-debate contract already tells the AI to push back on.

The alternative — writing a bespoke obligation table for intents — was rejected because it creates two contracts that must be kept in agreement forever. Any future edit to the dismissal-debate contract would silently apply to typed answers and not to intents.

One consequence is worth naming rather than discovering later: **the note attached to a `major` or `critical` dismissal is the opening argument, not the record.** The dismissal-debate contract requires the persisted `--reason` to reflect the debate's outcome, including counter-evidence the developer produced during it. A board note is the developer's opening position, so the reason written to the database may differ from what they typed on the board. That is correct and is not a bug.

### Intents are identified by the position IDs the developer already sees

Comment IDs in `tai` are per-target positions computed with `ROW_NUMBER()` at query time, not stable identities; a `tai triage forget` shifts every subsequent position. The intents file therefore records something that could, in principle, drift.

In practice it cannot, within the flow the board exists to serve. The flow is import → triage → fix → verify. Nothing in it deletes comments: re-import upserts by `external_refs` and appends, and accept / dismiss / complete never delete. The only operation that shifts positions is `tai triage forget`, a deliberate destructive gesture guarded by its own consent model, which nobody performs between opening a board and finishing the conversation it feeds.

Fingerprinting each intent against `file` / `lines` / `title`, or keying intents by `comment_external_refs.external_id`, were both considered and rejected as defences against a scenario the flow does not produce. Positions also keep one representation of a comment's identity across the board, the intents file, the conversation, and the CLI — introducing a second identifier that the AI must never see is a durable cost paid against a transient risk.

The intents file is stored at a scope-keyed path, so a board run for one PR cannot be read as intents for another — including under the slash command's stack mode, which runs the loop once per PR.

### The board is offered, never assumed

The board is worth opening when there is a bulk to act on and pure overhead when there is not. Below roughly five pending comments, opening a browser is slower than talking.

The AI offers the board when more than five comments remain pending *after* phase 1's investigation, and never launches it unprompted. Two properties follow: the threshold is evaluated against the comments that actually survive investigation, so a scope where the AI auto-completes most items does not trigger a pointless offer; and the default path for anyone who declines is unchanged.

### The board runs after investigation, not before

Phase 1 walks every pending comment looking for evidence it has already been fixed, and auto-completes what it finds. Rendering the board before that step would put comments the developer fixed last week in front of them, and would manufacture a contradiction the slash command would then need a rule for: the developer's intent says `accept`, and investigation then says "already fixed, completing it".

Running investigation first means the board only ever shows comments that are genuinely live, and that contradiction has no way to arise. The cost is that the developer waits while the AI reads git history before the browser opens, which the AI announces.

### A loopback listener on an ephemeral port, behind an unguessable path

Three constraints shaped the server:

- **Loopback only.** The board renders unreleased code review content. Binding anything other than `127.0.0.1` would expose it to the network.
- **Ephemeral port (`:0`), with the assigned URL printed.** A fixed port collides with whatever else the developer is running and produces a failure mode that has nothing to do with triage.
- **A 32-hex random path prefix.** Loopback is not a security boundary on a shared machine: any local process, including a browser tab on an unrelated site, can reach `127.0.0.1` on any port. The random path means an attacker must guess it to read the review contents or POST intents. This is roughly ten lines of code and closes the only realistic attack on the surface.

### Handing the intents back to the AI

The slash command launches the board as a background process. The board serves, blocks until submit, writes the intents artifact, and exits `0`. An agent harness that notifies on background-process exit — which the harness the slash command targets does — wakes the AI at exactly the right moment, with no polling contract to specify.

Not every harness offers that notification, and the plugin ships to whichever AI tool owns the target directory, so exit-notify cannot be the only mechanism. A harness without it polls `tai triage board intents`, which exits `TRIAGE_NO_INTENTS` while no artifact exists and emits the intents once one does. That exit is the not-yet signal, so polling needs no verb of its own.

A dedicated `tai triage board status` verb was considered for the polling case and rejected: it would carry its own output format, error contract and tests while adding no capability `board intents` does not already have.

A blocking foreground invocation whose stdout carries the intents was rejected: a developer working a 40-comment board takes ten to twenty minutes, which exceeds the per-command timeout of the harness the slash command targets, and a timeout would discard every decision they had made.

## Risks / Trade-offs

- **The browser page has no automated coverage.** Handler behaviour, the intents artifact, and `tai triage board intents` output are all testable and are covered. What a click does in the page is not. This is declared in `plugins/triage/test-cases.md` the same way the slash commands' conversational contracts already are, rather than papered over with a handler test that implies coverage it does not have.
- **Headless machines.** On a box with no browser, launching one fails. The board prints the URL and keeps serving rather than treating this as an error, so an SSH developer can forward the port.
- **A developer can abandon a board.** The process holds a port and blocks until submit or until it is killed. There is no timeout: an abandoned board is a stray process the developer kills, and inventing an expiry would risk discarding a half-finished pass.
- **Two surfaces now describe a comment.** The board's rendering and `tai triage show`'s markdown must stay in agreement, or the developer sees one thing on the board and the AI quotes another in conversation. Both read `listSQL` in `plugins/triage/internal/triage`, which its own comment calls the canonical SELECT for list and show, so there is one projection rather than two kept in step by discipline. A rendering scenario asserts every field that `tai triage show` renders appears in the served HTML, so a template that stops consuming part of that projection fails a test rather than silently showing less than the conversation quotes.

## Open Questions

None blocking. Two matters are deliberately deferred:

- Whether the board should offer any filtering or sorting control beyond the batch-first, severity-ordered layout the loop already mandates. Deferred until the fixed ordering is shown to be insufficient in use.
- Whether `complete` should be a fourth intent. It is omitted because phase 1's investigation already auto-completes what it can prove, and a developer's unverified "I already fixed that" is exactly the claim the conversation is good at probing.
