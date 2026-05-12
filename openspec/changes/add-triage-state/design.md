## Context

`add-storage-schema` defined the data; `add-import-command` defined the data ingress. Both produced rows in `pending` status. This proposal adds the surface that actually moves those rows through their lifecycle — the most-trafficked verbs in tai.

Several concerns pull in different directions:

- **AI ergonomics**: Claude needs the comment text in a stable, parseable markdown layout. Section names and order matter for downstream prompts.
- **Human ergonomics**: humans typing at a shell want compact IDs, sensible defaults, idempotent verbs.
- **Safety**: `tai forget` is destructive and must not delete data without consent. But it also runs in non-interactive automation (the `/tai:triage` skill calls it after the user confirms in conversation).
- **Coherence**: seven verbs with overlapping flags is a lot of surface. They MUST share a scope-resolution rule, an ID scheme, and an output style — or they'll feel like seven unrelated tools.

This document records the decisions that hold the seven verbs together. The corresponding `specs/triage/spec.md` carries the normative contracts.

## Goals / Non-Goals

**Goals:**

- One scope-resolution rule used by every verb (except `tai forget --repo`), with clear precedence and clear errors when ambiguous.
- Per-target sequential IDs that match the user's mental model and are stable in normal workflows.
- A markdown output for `tai show` that any future AI integration can parse without surprises.
- State transitions that allow users to change their minds without ceremony, while preserving the import layer's "frozen" contract.
- A `tai forget` consent model that errors loudly in automation rather than skip silently.

**Non-Goals:**

- A `tai resolve` / `tai update` verb to mutate enrichment fields. If a comment's text is wrong, the path is `tai forget --comment <id>` then re-import. Avoiding update-text verbs keeps the import layer the source of truth for comment content.
- Bulk operations beyond per-batch (e.g. `tai accept --severity critical` or "accept all pending"). Premature; the slash command iterates explicitly.
- Audit-log history of decisions. Each row stores its current state only.
- Cross-target operations (e.g. "list all comments across all my PRs"). Each invocation targets a single scope.
- A read-only "session" mode (e.g. a TUI). Verbs are one-shot.

## Decisions

### D1. Scope resolution — flag > current branch > error

The rule used by every verb except `tai forget --repo`:

```
1. If --pr <N> is provided:
      scope = (current repo, PR matching repo_id + number)
      Fail TRIAGE_NOT_FOUND if no such PR row.
2. Else if --branch <name> is provided:
      scope = (current repo, branch matching repo_id + name)
      Fail TRIAGE_NOT_FOUND if no such branch row.
3. Else (auto-detect from current git branch):
      a. Look up prs.head_branch == <current> AND repo_id == <current_repo>
      b. Look up branches.name == <current> AND repo_id == <current_repo>
      c. If exactly one matches → that's the scope.
      d. If both match → fail TRIAGE_AMBIGUOUS_SCOPE.
      e. If neither matches → fail TRIAGE_NO_SCOPE.
```

`--pr` and `--branch` are mutually exclusive. Providing both fails `TRIAGE_INVALID_FLAGS`.

**Alternatives considered:**

- Auto-detect by *most-recent activity* (last imported into) regardless of current branch. Less predictable; the current branch is the user's actual context.
- Prefer PR over branch when both match (instead of erroring). Tempting; rejected because the only way both match is if the user did a branch-scoped review *then* opened a PR. In that case they almost certainly want to merge the data — surfacing the conflict is more honest.

### D2. User-facing IDs are per-target row positions, computed at query time

Per-target sequential IDs (`1, 2, 3, …` per PR or branch) are surfaced everywhere a comment is named. They are computed at query time as:

```sql
ROW_NUMBER() OVER (PARTITION BY pr_id, branch_id ORDER BY id ASC)
```

Internal DB ids are never exposed. The mapping from user-facing position to internal id happens at the boundary of every read and every mutation.

Positions are stable as long as no comment is deleted from the same target. After a `tai forget --comment <id>`, positions shift. This is documented; the workflow that uses tai (import → triage → done) rarely deletes individual comments.

**Alternatives considered:**

- Store `target_seq` as a stored column. Stable across deletes. But requires schema change (delta to storage capability), and "stability across deletes" is a feature most users will never exercise.
- Expose internal DB ids directly. Ergonomic disaster — IDs become 482, 9847, etc. Users can't predict them.
- Hybrid: per-target positions for display, DB ids accepted with `#` prefix for unambiguous mutation. Marginal benefit; postpone until anyone asks.

### D3. Batch-wide operations are first-class

Each terminal-state verb (`accept`, `dismiss`, `complete`) supports `--batch <key>` as an alternative to a comment id. Behaviour:

- Resolve the batch within current scope (errors with `TRIAGE_NOT_FOUND` if absent).
- For every member comment whose current status differs from the requested target state: apply the transition. Members already in the target state are left untouched.
- After the per-member work, recompute the batch's `status`:
  - All members in target state → batch's status matches.
  - Members split between two terminal states (e.g. some accepted, some dismissed) → batch status `mixed`.
  - Members still in `pending` and others in a terminal state → batch status `mixed`.

`--batch` and `<id>` are mutually exclusive. Providing both fails `TRIAGE_INVALID_FLAGS`.

**Alternatives considered:**

- Batch operations as a separate verb (`tai batch-accept B1`). Adds ceremony. Sharing flags is friendlier.
- Refuse to apply if any member is already in a different terminal state. Conservative but defeats the point — the user explicitly asked.

### D4. State machine is liberal; resolution/reason fields follow the verb

Any current status → any new status via the corresponding verb:

| Verb | Sets status to | Touches |
|---|---|---|
| `tai accept` | `accepted` | clears `dismissed_by`/`dismiss_reason`; sets `resolution` if `--resolution` provided |
| `tai dismiss` | `dismissed` | clears `resolution`; sets `dismissed_by` and `dismiss_reason` (REQUIRED) |
| `tai complete` | `completed` | clears `dismissed_by`/`dismiss_reason`; sets `resolution` if `--resolution` provided |

Self-transitions are idempotent successes (`tai accept` on an already-accepted row updates `resolution` if provided and otherwise is a no-op).

**Why clear the unrelated fields:** carrying stale `dismiss_reason` on an `accepted` row would confuse `tai show`. The row should tell the truth about its current state.

**Alternatives considered:**

- Refuse non-pending → X transitions; require `--force`. Hostile to natural workflows. The user changing their mind is the most natural thing in the world.
- Preserve historical fields (carry `dismiss_reason` even after accept). Confusing.

### D5. Markdown output contract — stable sections, ordered, ready for LLM context

Each `tai show` block uses this exact shape:

```markdown
# <repo> <target-label> — comment <pos> of <total>

**Severity:** <severity>  **Category:** <category>  **Status:** <status>
**File:** `<file>:<lines>`
**Source:** <source>
**Batch:** <batch_key> — <batch_title>     ← present iff batch_id is set

## Title
<comment.title>

## Description
<comment.description>

## Why fix it
<comment.why_fix>

## Suggested fix
<comment.suggested_fix>

## What happens if you don't fix it
<comment.consequences>

## Resolution                ← present iff status in (accepted, completed) AND resolution non-null
<comment.resolution>

## Dismissed because         ← present iff status == dismissed
<comment.dismiss_reason> (by <comment.dismissed_by>)
```

`tai show --all` produces one block per comment, sorted by target position, joined with:

```
\n\n---\n\n
```

between blocks (a horizontal rule on its own line, blank line above and below).

The section names and order are part of the spec contract. Future proposals MAY add new sections at the end but MUST NOT rename or reorder existing ones.

**Alternatives considered:**

- JSON output. Easier for machines, worse for Claude. Markdown is the lingua franca of LLM context.
- A `--json` flag. Rejected per the foundation decision to keep output human-readable everywhere.
- A `--compact` flag that drops sections. Premature; if users want, they can `grep` the markdown.

### D6. `tai status` is a compact dashboard

Output:

```
Repo: <owner>/<name>
Scope: PR #<number> — <title> (branch: <head_branch>)

Counts:
  Total:      <N>
  Pending:    <P>
  Accepted:   <A>
  Dismissed:  <D>
  Completed:  <C>

Batches: <count>
  <key> (<n> comments — <status>) — <title>
  …

[exit 0]
```

For branch targets the `Scope:` line reads `branch <name>` and omits `(branch: …)`.

When the resolved scope has zero comments (target rows exist but no comments imported), the counts block reads `Total: 0`.

### D7. `tai forget` is single-mode and destructive

Exactly one of `--repo`, `--pr`, `--branch`, `--comment` is required. Providing zero or more than one fails `TRIAGE_INVALID_FLAGS`.

Each mode resolves a delete plan, shows a summary, prompts, and (on consent) deletes inside one transaction:

```
You're about to delete:
  • acme/app PR #142 — feat: add OAuth2 token refresh flow
  • 12 comments
  • 1 batch
  • 24 external references

This cannot be undone. Continue? [y/N]
```

`--yes` flag, or `TAI_ACCEPT_DESTRUCTIVE` env var with truthy semantics, skip the prompt. Non-interactive stdin without consent fails `TRIAGE_CONFIRMATION_REQUIRED` (exit 1) — destructive ops fail loudly.

`tai forget --repo <owner/name>` is the only mode that does NOT require repo context to be resolvable (it carries its own identity). It MAY be run from anywhere on the filesystem, including outside any git repository. The other three modes require resolved scope per D1.

**Alternatives considered:**

- A single `tai forget <target-expression>` that auto-detects intent. Too magical.
- Soft-delete (mark `deleted_at`). YAGNI; user wants a hard delete with cascade.

### D8. Output formatting — `tai list` uses a compact table

```
Repo: acme/app   Scope: PR #142 — feat: add OAuth2 token refresh flow

  ID  SEV    STATUS     BATCH  FILE                      TITLE
  1   crit   pending    B1     src/api/auth.ts:15-29     Replace execSync with execFileSync to prevent shell injection
  2   crit   accepted   B1     src/utils/fetch.ts:41-48  Replace execSync with execFileSync in fetch utility
  3   maj    pending    -      src/api/auth.ts:51-57     Handle expired refresh token gracefully
```

Severity abbreviations: `crit`, `maj`, `min`, `nit`. Status displays the full word. `TITLE` is truncated to fit the terminal width with `…` when needed; falls back to 80 chars when width can't be detected. The table heads are always present.

**Alternatives considered:**

- Full-width title lines, no truncation. Wraps awkwardly in narrow terminals.
- JSON output for scripting. Rejected per the foundation contract.

### D9. Reuse `isTruthyEnv` and `--yes`-style affordances

`TAI_ACCEPT_DESTRUCTIVE` uses the same `isTruthyEnv` helper introduced by `add-install-command`. Future destructive verbs (none planned) would reuse the env var name; `--yes` is the standard per-invocation override across all destructive verbs.

### D10. `--status` filter on read verbs

`tai list` and `tai show --all` accept zero or more `--status <state>` flags. Multiple values combine via OR. When no flag is given, all statuses are shown.

The filter is on the read verbs only — `tai show <id>` (single-comment) never applies the filter because it's already addressing a specific row by position.

Motivation: the post-triage workflow (`/tai:verify`, manual fix-and-prune cycles) is dominated by "show me everything in status X" queries. Without a filter, callers have to pipe through `awk` or write their own SQL.

**Alternatives considered:**

- A boolean `--pending-only` shortcut. Tempting; rejected because the symmetric "show me everything except pending" still needs a verbose flag. A general `--status` is uniform.
- A query-string-style `--filter status=accepted`. Over-general; we only filter on status today.

### D11. `--status` modifier on `tai forget`

`tai forget` accepts `--status <state>` (repeatable) as a modifier on `--pr`, `--branch`, `--repo`, and `--batch`. The modifier narrows the deletion to comments matching the supplied statuses; parent rows (PR/branch/repo/batch) are preserved.

Combining `--status` with `--comment` is rejected (a specific comment ID is already a precise target). When `--status` is combined with `--batch`, the batch row is preserved and its status recomputes against the surviving members.

The motivating workflow: after `/tai:verify` (or manual fix work) bumps a slug of comments to `completed`, the user wants to prune them without losing the surrounding PR/branch/repo context. `tai forget --pr 142 --status completed --yes` is the canonical post-fix cleanup.

The consent model is unchanged — `--yes`, `TAI_ACCEPT_DESTRUCTIVE`, and interactive prompt all apply identically. The summary block names the count of matching comments (not the parent row) so the user knows exactly what will go.

**Alternatives considered:**

- A separate `tai prune` verb. Adds a verb that does what an existing flag combination already covers.
- `--status` on `--comment` interpreted as "only delete if this comment matches". Allowed in theory; rejected because users selecting by ID know what they're deleting — the filter adds confusion without value.

## Risks / Trade-offs

- **[Per-target ID shift after `tai forget --comment`]** Documented limitation. The workflow rarely uses single-comment forget; users will see this in edge cases.

- **[Liberal state machine could mask thrashing]** A user who flips a comment between accepted and dismissed many times leaves no trace. → Audit log is a non-goal for v1; if it's ever needed, a parallel `comment_state_transitions` table can be added without breaking existing rows.

- **[Markdown shape is now an API contract]** Future tweaks risk breaking `/tai:triage` or any user automation that parses `tai show`. → The spec names the contract; section names are reserved. Changes go through OpenSpec.

- **[`tai forget --repo` can be run anywhere]** A copy-paste from another shell could wipe the wrong repo's data. → The prompt names the exact repo being deleted. `--yes` and `TAI_ACCEPT_DESTRUCTIVE` are both opt-in.

- **[Auto-detect by current git branch is git-dependent]** If the user `cd`s elsewhere or is in a detached HEAD state, auto-detect fails. → Errors with `TRIAGE_NO_SCOPE` and points the user at `--pr`/`--branch`. Documented in the "What to do" block.

- **[Batch status `mixed` requires recompute]** Every batch mutation triggers a recompute of the batch's status across its members. → Cheap (batches have ~10 members at most); not a concern.

- **[No bulk verbs]** A user with 200 pending comments cannot accept them all with one command. → The slash command (`/tai:triage`) iterates explicitly. Humans who want a script can loop in their shell.

## Migration Plan

No prior triage state to migrate; this is the first proposal that mutates row status post-import. The first user invoking `tai accept`/etc. simply writes a new state on a pending row.

## Open Questions

(None remaining.)
