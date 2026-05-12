## Context

`tai import` is the data ingestion boundary between the unstructured outside world and tai's strict relational schema. Two-and-a-half decisions drove the shape:

1. tai's CLI stays out of the network and out of Claude conversations. The slash command does both; the CLI gets a fully-validated JSON payload on stdin.
2. The five mandatory enrichment fields (`severity`, `category`, `why_fix`, `suggested_fix`, `consequences`) are non-negotiable at the schema layer; the CLI rejects any comment missing them.
3. Triage decisions, once recorded, are sacred. Re-import refreshes pending rows but never touches accepted, dismissed, or completed ones — a user's "I decided about this" should not silently change underneath them.

This document records the JSON schema, the upsert state machine, and the boundary between the CLI verb and the bundled `/tai:import` Claude slash command. The corresponding `specs/import/spec.md` carries the normative contract.

## Goals / Non-Goals

**Goals:**

- A JSON schema strict enough that any payload accepted by the CLI is guaranteed to populate the database without further validation.
- Upsert semantics that make re-import safe: triage state is preserved, new comments appear, edited upstream content flows through to pending rows.
- A clear contract for the slash command — what it pulls, how it enriches, what JSON it produces — so the command is replaceable without changing the CLI.
- CLI invocations that can be replayed from a captured stdin payload (useful for tests, debugging, and humans who want to script imports without Claude).

**Non-Goals:**

- Multi-target payloads. One invocation = one PR or branch. Stack mode is the slash command's loop, not the CLI's.
- The CLI making network calls. No `gh api`, no `st_reviews`, no GitHub clients. Adding any of these would mean the CLI also needs authentication state, error handling for rate limits, etc. — re-implementing what `gh` already does.
- The CLI making Claude calls. Enrichment is the slash command's job.
- Cross-PR deduplication. Each `(target, comment)` is independent; the same external comment cannot appear under two targets.
- Schema versioning of the JSON payload. The schema is stable; if it ever changes, the change is part of an OpenSpec proposal that simultaneously updates the bundled `/tai:import` command's frontmatter `version`. Past versions of the slash command will keep producing the old shape and will fail validation — fine, because users on the old slash command will be on an old CLI too.

## Decisions

### D1. CLI accepts only `tai import -` (stdin), no positional shorthand

The verb name `import` is used only with `-` as its sole argument, signalling "read from stdin". Calling `tai import` with any other argument (a PR number, a URL, a filename) fails with usage error.

**Alternatives considered:**

- `tai import 142` that does its own `gh api` calls. Doubles the CLI's surface area, adds `gh`/network dependencies into the binary's contract, duplicates the slash command's collection logic. Rejected.
- `tai import < file.json` (no explicit `-`). Works equivalently, since `urfave/cli` treats stdin as default if no positional given. We keep the explicit `-` as a documentation hint that "this command reads stdin" — `tai import` alone shows the help.

### D2. JSON schema is one target per payload; stack mode loops

A payload describes exactly one PR or one branch:

```json
{
  "repo": "acme/app",
  "target": {
    "kind": "pr",
    "pr": {
      "number": 142,
      "title": "feat: add OAuth2 token refresh flow",
      "url": "https://github.com/acme/app/pull/142",
      "head_branch": "feat/oauth"
    }
  },
  "batches": [
    { "batch_key": "B1", "title": "Replace execSync with execFileSync" }
  ],
  "comments": [
    {
      "external_refs": [
        { "kind": "github-pr-comment", "id": "12345", "reviewer": "coderabbit" }
      ],
      "severity": "critical",
      "category": "security",
      "file": "src/api/auth.ts",
      "lines": "15-29",
      "source": "coderabbit",
      "title": "Replace execSync with execFileSync to prevent shell injection",
      "description": "execSync interpolates user input into a shell string. Use execFileSync to pass arguments as an array without shell interpretation.",
      "why_fix": "execSync runs its argument through a shell, so any value containing shell metacharacters is interpreted.",
      "suggested_fix": "Replace execSync(`git config ${k} ${v}`) with execFileSync('git', ['config', k, v]); rerun unit tests.",
      "consequences": "An attacker who can influence k or v (e.g. via a malicious branch name) gains arbitrary command execution in the build environment.",
      "batch_key": "B1"
    }
  ]
}
```

For `target.kind = 'branch'`, the `pr` object is absent and a `branch` object with `{ name }` is present instead.

Stack imports are repeated invocations of `tai import -`, one per PR. The slash command writes the loop; the CLI never sees a multi-target shape.

**Alternatives considered:**

- A top-level `targets[]` array supporting multi-PR payloads in one invocation. Saves a few subprocess launches but tangles transaction semantics: do we commit per-target or all-or-nothing? Per-target is the right answer (a failing PR shouldn't roll back the others), at which point the CLI is just doing what the slash-command loop would. Multi-target in one payload is added complexity for no behavioural win.

### D3. Repo identity comes from the JSON, not `git config`

`tai import` does NOT call `git config --get remote.origin.url`. The JSON's `repo` field is the source of truth for which repo this data is for. This makes `tai import` runnable from anywhere — useful for tests and replayable in CI.

The foundation's `--repo` flag is rejected (usage error) if combined with `tai import`, because there's nothing to override; the JSON contains the answer.

**Alternatives considered:**

- Resolve repo from `git config` if `repo` is omitted from JSON. Would couple the CLI to filesystem state and re-introduce the `REPO_NOT_FOUND` error path that import is otherwise free of. Rejected — the JSON is self-describing.

### D4. Upsert is keyed by `external_refs`; triage state is sacred

For each incoming comment:

1. For each entry in `external_refs`, look up `comment_external_refs(source_kind, external_id)`.
2. If all resolving refs point to the same existing `comments.id`, OR exactly one ref resolves and the rest are new, use that comment:
   - If existing comment's `status == 'pending'`: refresh enrichment fields (`title`, `description`, `severity`, `category`, `file`, `lines`, `source`, `why_fix`, `suggested_fix`, `consequences`) and `batch_id`. Add any new external_refs.
   - If existing comment's `status != 'pending'`: leave enrichment and batch_id untouched. Add any new external_refs only.
3. If no refs resolve: insert a new comment with `status = 'pending'`, then insert every external_ref.
4. If refs resolve to *multiple distinct comments*: exit with `IMPORT_AMBIGUOUS_REFS`. The slash command can detect this and ask the user to manually reconcile.

The transaction commits atomically — if any comment in the payload fails (e.g. constraint violation), the whole import rolls back.

**Alternatives considered:**

- Always refresh enrichment regardless of status. Surprises the user: "I accepted that comment yesterday with a specific resolution; today its description silently changed because the reviewer edited their message." Bad.
- Refresh-on-status-pending behind a `--refresh-pending=false` flag. Possibly useful, but YAGNI for v1; the default behaviour is the safe one.
- Heuristic dedup on `(title, file, lines)` when refs don't match. Implicit; better that the slash command does explicit dedup and stamps refs accordingly.

### D5. Batches are upserted by `(target, batch_key)`; status is not touched

A `batches` row is keyed by `(pr_id, batch_key)` or `(branch_id, batch_key)`. On re-import:

- If a batch with the same key exists, its `title` is updated; `status` is left as-is.
- If the key is new, the batch is inserted with `status = 'pending'`.

Batch `status` is the province of the triage state machine (specified in `add-triage-state`), not import. A batch marked `accepted` yesterday stays `accepted` after a re-import.

### D6. Empty payload is a successful no-op

`{"repo": "…", "target": {…}, "comments": [], "batches": []}` is valid. The repo and target rows are upserted (creating them if absent) and the CLI returns success.

This matters because the slash command's collection step might find nothing on a re-import after all comments have already been triaged. The CLI shouldn't punish a no-op.

### D7. CLI validation is strict, exhaustive, and runs before any writes

The validator walks the entire payload before opening a transaction. Errors collected during validation are reported together in the error message:

```
Error: 3 problems with the JSON payload:

  comments[0].why_fix: required field is empty
  comments[1].severity: "urgent" is not one of (critical, major, minor, nitpick)
  comments[2].external_refs: required field is missing

What to do:
  • The /tai:import command emitted incomplete data. Re-run /tai:import,
    or fix the JSON manually if you piped it from somewhere else.

[exit 3: IMPORT_SCHEMA_INVALID]
```

Reporting all errors at once helps the slash command surface every issue to the user in one round.

**Alternatives considered:**

- Fail on the first error. Saves a few microseconds; produces worse UX when payloads have multiple issues.

### D8. Output on success is a one-block summary

```
$ tai import - < /tmp/payload.json
Imported acme/app PR #142 (12 comments, 3 batches)
  Inserted:  10 new comments
  Updated:    2 existing comments (pending)
  Frozen:     0 comments left untouched (already triaged)
  Refs added: 3 external refs attached to existing comments
  Batches:    2 inserted, 1 updated
[exit 0]
```

The slash command parses these counts to tell the user what happened. The "Frozen" line surfaces when an already-triaged comment was re-pulled — useful context.

### D9. The bundled `/tai:import` slash command with two collection modes

The bundled markdown body drives a conversation. The invocation form picks both a scope and a **collection mode**:

| Invocation | Scope | Collection mode |
|---|---|---|
| `/tai:import` | current PR | remote |
| `/tai:import <pr-number-or-url>` | that PR | remote |
| `/tai:import stack` | every PR trunk → current | remote (one payload per PR) |
| `/tai:import branch <name> [--from <path>]` | that branch | manual |

**Remote mode** pulls raw comments from the network:

- Prefer `st_reviews(scope='current'|'to-current')` when staccato MCP is connected.
- Otherwise `gh api repos/{o}/{n}/pulls/{n}/{comments,reviews}` plus `issues/{n}/comments`.
- If neither is available, exit early with installation guidance.

**Manual mode** sources content from whatever the user has at hand, in this order of preference:

1. **File path** — when `--from <path>` (or a trailing positional path) is given, the slash command reads the file. Markdown is parsed as free-form review text; JSON is parsed as a (possibly-enriched) tai payload; other plain text is best-effort.
2. **Current conversation context** — when no path is given, the slash command looks for review content already present in the Claude conversation (e.g. a prior AI's review output).
3. **Prompt the user** — when neither yields content, the slash command asks the user to paste, name a path, or describe the review.

The motivating flows for manual mode:

- A user runs an AI review against their staged changes (inside or outside this conversation) and wants to capture the output against a branch that has no GitHub PR.
- A teammate pastes a code review into chat and the user wants it filed.
- A linter / static-analysis tool's output is on disk; the user wants it ingested.
- An audit report exists in Markdown.

For all sub-paths the slash command:

- Parses the content into discrete comments (file, lines, draft title/description).
- Enriches with the five mandatory fields, confirming each draft with the user before piping. The one exception: when the source is a JSON file already matching tai's payload schema, the slash command MAY surface a single confirmation summary instead of per-field prompts.
- Generates `external_refs` with `kind = "manual"` and a deterministic `id` (sha256 of `file + lines + title`, truncated). Deterministic IDs make re-imports idempotent. When the source JSON already supplies external_refs, those are passed through verbatim.

The slash command MUST NOT mix modes: a `branch` invocation never calls `gh`, and a PR invocation never relies on a file path or conversation context for raw content.

For both modes:

- Comments are enriched in conversation; the user confirms each draft.
- Batches are spotted by shared corrective actions (per the existing `pr-review-triage` skill's heuristic).
- The assembled JSON payload is piped to `tai import -`.
- On `IMPORT_AMBIGUOUS_REFS`, the conflict is surfaced and the user reconciles.

The slash command is the only place tai opines about what makes a "good" enrichment. The CLI's job is purely to enforce structural validity.

**Alternatives considered:**

- A separate `/tai:capture` slash command for manual mode. Same shape, same destination; splitting just doubles the surface area users have to remember.
- Manual mode locked to conversation context only. Too narrow — users have reviews in files, pasted from chat tools, output from local linters, etc.
- Branch mode that also calls `gh` to enrich (e.g. blame, file history). Premature; the source of the review usually has full file context already.
- Non-deterministic external IDs in manual mode (e.g. UUIDs at import time). Breaks idempotent re-import; users re-running the same import would get duplicates.

### D10. CLI does not run migrations on `tai import` differently from any other DB-touching verb

Per `add-storage-schema`, every verb that opens the database runs the migration runner first. `tai import` is no exception. This is mentioned only to flag that the very first user who runs `tai import` will see the database file get created and migrated transparently.

## Risks / Trade-offs

- **[Stack imports are not atomic across PRs]** A stack of 4 PRs imports as 4 transactions. If the third one fails, the first two are committed and the fourth never runs. The slash command surfaces this and the user re-runs the import for the failed PR. → Acceptable; the per-PR transaction is the right granularity. Cross-PR rollback would require a single mega-transaction holding open for the duration of all `gh api` calls, which is worse.

- **[Validator must keep up with schema changes]** Adding a new column to `comments` means updating the JSON validator AND the bundled slash command's prompt. Easy to forget. → Mitigated by making schema fields enums in code (severity, category, status) so the validator and the migration share the same source.

- **[Ambiguous refs can be hard to recover from]** If refs resolve to two existing comments because of a prior dedup that's no longer accurate, the user has to manually delete one (likely via `tai forget` on the wrong row) and re-import. → Documented in the spec; in practice this is rare and the error message explains it.

- **[Frozen comments hide upstream edits]** Once a comment is `accepted`/`dismissed`/`completed`, an upstream edit by the reviewer is silently ignored. → This is the intentional contract: triage state is sacred. A future `tai refresh <id>` verb could force-refresh a single row if anyone asks; not in v1.

- **[Empty no-op upsert touches `created_at`]** Inserting/updating the target row updates its `created_at`/`updated_at`. → For PRs/branches, we use `INSERT OR IGNORE` semantics that only set `created_at` on first insert. Documented in the spec.

- **[Slash command depends on `gh` or `st_reviews`]** If neither is available, the command exits early. → The command's body explicitly checks for both at start and tells the user how to install `gh` if missing.

- **[JSON payload size]** For large PRs (50+ comments), the payload could exceed typical command-line buffers. We pipe via stdin (not as an argument), so OS argv limits don't apply. SQLite handles the inserts trivially. → Not a real concern.

## Migration Plan

There is no prior import state; this is the first verb that writes review data. The very first invocation of `tai import` on a user's machine creates `tai.db` (via the migration runner inherited from `add-storage-schema`) and inserts the first rows.

## Open Questions

(None remaining.)
