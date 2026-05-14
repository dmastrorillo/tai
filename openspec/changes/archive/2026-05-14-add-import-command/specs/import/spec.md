## ADDED Requirements

### Requirement: `tai import -` reads JSON from stdin

The system SHALL provide a `tai import` subcommand that accepts a single positional argument `-`, signalling that the JSON payload is read from stdin.

Invoking `tai import` with no argument MUST exit `1` with `UNKNOWN_SUBCOMMAND` (no implicit help). Invoking with any positional argument other than `-` MUST also exit `1` with `UNKNOWN_SUBCOMMAND` (e.g. "tai import expects '-' to read from stdin"). This is a deliberate departure from urfave/cli's default subcommand-without-args behaviour: import only makes sense with a payload, so silent help is misleading.

`tai import` MUST NOT read from the network. It MUST NOT shell out to `gh`, `git`, `st`, or any other external command.

#### Scenario: tai import - reads stdin

- **WHEN** `tai import -` is invoked with a valid JSON payload piped to stdin
- **THEN** the CLI parses, validates, and persists the payload
- **AND** exits with code `0`

#### Scenario: tai import with positional argument fails

- **WHEN** `tai import 142` is invoked
- **THEN** the CLI exits with code `1` and a usage error

### Requirement: `tai import` is repo-independent

The system SHALL classify `tai import` as repo-independent per the `cli-framework` carve-out. The `--repo` flag SHALL be rejected if combined with `tai import`, exiting `1` with a usage error.

Repo identity for the import is taken exclusively from the JSON payload's `repo` field. `tai import` MUST NOT call `git config` or any other process to resolve repo context.

#### Scenario: tai import succeeds outside a git repo

- **WHEN** `tai import -` is invoked from a directory that is not inside any git repository
- **AND** the JSON payload contains `"repo": "acme/app"`
- **THEN** the CLI succeeds and persists the data under `acme/app`

#### Scenario: --repo rejected with tai import

- **WHEN** `tai import - --repo acme/other` is invoked
- **THEN** the CLI exits with code `1` and a usage error explaining that `--repo` is not accepted by `tai import`

### Requirement: JSON payload schema

The system SHALL accept JSON payloads that match this top-level shape:

```
{
  "repo":     <string, format "<owner>/<name>", required>,
  "target":   <Target, required>,
  "batches":  <array of Batch, optional, default []>,
  "comments": <array of Comment, optional, default []>
}
```

Where `Target` is exactly one of two shapes:

```
PR target:
{
  "kind": "pr",
  "pr": {
    "number":      <integer, required, > 0>,
    "title":       <string, required, non-empty>,
    "url":         <string, required, non-empty>,
    "head_branch": <string, required, non-empty>
  }
}

Branch target:
{
  "kind": "branch",
  "branch": {
    "name": <string, required, non-empty>
  }
}
```

A target with `kind = "pr"` MUST contain a `pr` object and MUST NOT contain a `branch` object, and vice versa.

`Batch` shape:

```
{
  "batch_key": <string, required, non-empty>,
  "title":     <string, required, non-empty>
}
```

`Comment` shape:

```
{
  "external_refs": <array of ExternalRef, required, length >= 1>,
  "severity":      <string, required, one of: critical, major, minor, nitpick>,
  "category":      <string, required, one of: security, correctness,
                                              feature-regression, code-quality,
                                              performance, testing>,
  "file":          <string, required, non-empty>,
  "lines":         <string, required, non-empty>,
  "source":        <string, required, non-empty>,
  "title":         <string, required, non-empty>,
  "description":   <string, required, non-empty>,
  "why_fix":       <string, required, non-empty>,
  "suggested_fix": <string, required, non-empty>,
  "consequences":  <string, required, non-empty>,
  "batch_key":     <string, optional; if present, MUST match a batch_key in
                            the payload's batches[]>
}
```

`ExternalRef` shape:

```
{
  "kind":     <string, required, non-empty>,
  "id":       <string, required, non-empty>,
  "reviewer": <string, optional>
}
```

Any unknown top-level or nested key MUST be rejected (strict mode — no silent ignoring).

#### Scenario: Well-formed PR payload accepted

- **WHEN** a payload with `kind: pr`, a complete `pr` object, and well-formed comments is piped to `tai import -`
- **THEN** the CLI succeeds

#### Scenario: Well-formed branch payload accepted

- **WHEN** a payload with `kind: branch` and a `branch.name` is piped
- **THEN** the CLI succeeds

#### Scenario: target with both pr and branch rejected

- **WHEN** a payload's `target` contains both a `pr` and a `branch` object
- **THEN** the CLI exits `3` with `IMPORT_SCHEMA_INVALID`

#### Scenario: target with neither pr nor branch rejected

- **WHEN** a payload's `target.kind = pr` but `target.pr` is absent
- **THEN** the CLI exits `3` with `IMPORT_SCHEMA_INVALID`

#### Scenario: Missing enrichment field rejected

- **WHEN** a comment is missing `why_fix`
- **THEN** the CLI exits `3` with `IMPORT_SCHEMA_INVALID`

#### Scenario: Invalid severity rejected

- **WHEN** a comment has `severity: "urgent"`
- **THEN** the CLI exits `3` with `IMPORT_SCHEMA_INVALID`

#### Scenario: Empty external_refs rejected

- **WHEN** a comment has `external_refs: []`
- **THEN** the CLI exits `3` with `IMPORT_SCHEMA_INVALID`

#### Scenario: Unknown field rejected

- **WHEN** a comment contains a key not listed in the schema (e.g. `"priority": "p0"`)
- **THEN** the CLI exits `1` with `IMPORT_INVALID_JSON` (the strict decoder rejects unknown keys at decode time, before schema validation runs)

#### Scenario: batch_key references unknown batch rejected

- **WHEN** a comment has `batch_key: "B1"` but no batch with `batch_key: "B1"` appears in the payload's `batches`
- **THEN** the CLI exits `3` with `IMPORT_SCHEMA_INVALID`

#### Scenario: Malformed JSON

- **WHEN** stdin contains malformed JSON
- **THEN** the CLI exits `1` with `IMPORT_INVALID_JSON`

### Requirement: All validation errors reported in one message

The system SHALL collect every validation error in a payload before reporting. When validation fails, the error message MUST list each violation on its own line, with a JSON-Pointer-style path indicating where in the payload the violation occurred.

#### Scenario: Multiple violations reported together

- **WHEN** a payload has three distinct schema violations
- **THEN** stderr's error message lists all three
- **AND** the CLI exits once with `IMPORT_SCHEMA_INVALID`

### Requirement: Upsert by external_refs

The system SHALL upsert comments using `external_refs` as the natural key. For each incoming comment:

1. Look up every `external_refs[i]` in `comment_external_refs(source_kind, external_id)`. Collect the distinct `comment_id` values returned.
2. If zero comment_ids resolved: insert a new comment with `status = 'pending'`; then insert every `external_refs[i]` into `comment_external_refs`.
3. If exactly one `comment_id` resolved:
   - If the existing comment's `status` is `pending`: UPDATE the comment's enrichment fields (`severity`, `category`, `file`, `lines`, `source`, `title`, `description`, `why_fix`, `suggested_fix`, `consequences`) and `batch_id` to match the incoming payload. Then INSERT any `external_refs[i]` not yet present.
   - If the existing comment's `status` is `accepted`, `dismissed`, or `completed`: leave the comment's enrichment fields and `batch_id` unchanged. INSERT any `external_refs[i]` not yet present.
4. If more than one distinct `comment_id` resolved: exit `3` with `IMPORT_AMBIGUOUS_REFS`. The error message MUST list the conflicting comment IDs.

All of the above SHALL run inside a single transaction. If any step fails, the transaction MUST roll back and no changes persist.

#### Scenario: New comment inserted

- **WHEN** a comment's external_refs do not match any existing rows
- **THEN** a new `comments` row is inserted with `status = 'pending'`
- **AND** all external_refs are inserted

#### Scenario: Pending comment is refreshed

- **WHEN** a comment's external_refs match an existing row whose `status = 'pending'`
- **AND** the payload contains a different `description` than what's stored
- **THEN** the existing row's `description` is updated to match the payload
- **AND** the row's `status` remains `pending`

#### Scenario: Accepted comment is frozen

- **WHEN** a comment's external_refs match an existing row whose `status = 'accepted'`
- **AND** the payload contains a different `description`
- **THEN** the existing row's `description` is NOT updated
- **AND** any new external_refs in the payload are still attached to the row

#### Scenario: Ambiguous refs rejected

- **WHEN** a comment's external_refs resolve to two distinct existing comment IDs
- **THEN** the CLI exits `3` with `IMPORT_AMBIGUOUS_REFS`
- **AND** stderr names both conflicting comment IDs

#### Scenario: Transaction rolls back on failure

- **WHEN** a multi-comment payload is imported and the third comment violates a constraint
- **THEN** no rows from this payload persist after the failure
- **AND** the CLI exits `3` with `DB_CONSTRAINT_VIOLATION`

### Requirement: Batch upsert

The system SHALL upsert batches keyed by `(target, batch_key)`. For each batch in `batches[]`:

- If a row exists for the same target with the same `batch_key`: UPDATE its `title` to match the payload. Leave `status` unchanged.
- Otherwise: INSERT a new batch row with `status = 'pending'`.

Comments referencing a batch via `batch_key` SHALL have their `batch_id` set to the resolved batch row.

#### Scenario: Existing batch title updated

- **WHEN** a batch with `(pr_id=1, batch_key='B1', title='old title')` exists
- **AND** the payload contains the same batch with `title='new title'`
- **THEN** the row's `title` is updated to `'new title'`
- **AND** the row's `status` is unchanged

#### Scenario: New batch inserted

- **WHEN** the payload contains a batch_key not present in the database
- **THEN** a new `batches` row is inserted with `status = 'pending'`

### Requirement: Repo and target rows are upserted

The system SHALL ensure that the payload's `repo` and `target` are represented in the database before processing comments. Repo and target upserts use `INSERT OR IGNORE` semantics: existing rows are unchanged; missing rows are inserted with the payload's values.

For PR targets: `prs.title`, `prs.url`, and `prs.head_branch` are written ONLY on insert. They are NOT updated on subsequent imports against the same `(repo, number)`. (Updating titles/URLs is out of scope for v1; if a PR is renamed upstream the user can `tai forget` and re-import.)

#### Scenario: Repo created on first import

- **WHEN** `tai import -` runs for `repo=acme/app` and no `repos` row exists
- **THEN** a `repos` row is inserted with `owner_name='acme/app'`

#### Scenario: Repo unchanged on re-import

- **WHEN** the `repos` row for `acme/app` exists with `created_at=T1`
- **AND** a re-import for the same repo runs at `T2`
- **THEN** the row's `created_at` is unchanged

#### Scenario: PR title not updated on re-import

- **WHEN** an existing `prs` row has `title='old'`
- **AND** the payload contains the same `(repo_id, number)` with `title='new'`
- **THEN** the row's `title` remains `'old'`

### Requirement: Empty payload is a successful no-op

The system SHALL accept payloads with `comments: []` and `batches: []` as valid. The repo and target rows are still upserted; the CLI exits `0` and prints the standard success header `Imported <owner>/<name> <target-label> (0 comments, 0 batches)` with no per-counter lines.

#### Scenario: Empty payload succeeds

- **WHEN** a valid payload with no comments and no batches is piped to `tai import -`
- **THEN** the CLI exits `0`
- **AND** the summary header reads `Imported <owner>/<name> <target-label> (0 comments, 0 batches)`
- **AND** the repo and target rows exist in the database after the call

### Requirement: Success output

On successful import the CLI SHALL write a single-block summary to stdout:

```
Imported <owner>/<name> <target-label> (<N> comments, <M> batches)
  Inserted:  <I> new comments
  Updated:   <U> existing comments (pending)
  Frozen:    <F> comments left untouched (already triaged)
  Refs added: <R> external refs attached to existing comments
  Batches:   <BI> inserted, <BU> updated
[exit 0]
```

Where `<target-label>` is `PR #<number>` for PR targets and `branch <name>` for branch targets.

Lines whose counts are zero MAY be omitted (except the header line, which is always present).

#### Scenario: PR success summary header

- **WHEN** a payload with `kind=pr, number=142` imports successfully
- **THEN** the summary's first line is `Imported acme/app PR #142 (…)`

#### Scenario: Branch success summary header

- **WHEN** a payload with `kind=branch, name=feat/x` imports successfully
- **THEN** the summary's first line is `Imported acme/app branch feat/x (…)`

### Requirement: Bundled `/tai:import` slash command contract

The bundled `/tai:import` slash command's markdown body SHALL drive a Claude conversation with the following obligations:

1. **Scope resolution and collection mode**. Each invocation resolves to a scope AND a collection mode. The collection mode determines where the raw review content comes from:
   - `/tai:import` (no arg) → current PR scope; collection mode `remote`.
   - `/tai:import <pr-number-or-url>` → single PR scope; collection mode `remote`.
   - `/tai:import stack` → every PR from trunk to the current branch, ancestor-first; collection mode `remote`.
   - `/tai:import branch <name>` → branch scope; collection mode `manual`. Optional positional `--from <path>` or trailing positional argument naming a file may be supplied.
2. **Remote collection mode**. The slash command MUST pull raw comments from one of two backends:
   - Preferred: the staccato `st_reviews` MCP tool (`scope='current'` or `scope='to-current'`).
   - Fallback: `gh api repos/{owner}/{name}/pulls/{n}/comments`, `…/reviews`, and `repos/{owner}/{name}/issues/{n}/comments`.
   If neither backend is available, the slash command MUST exit early with a user-facing message telling the user to install `gh` or connect staccato.
3. **Manual collection mode**. The slash command MUST source raw review content from one of the following, in order of preference:
   - **File path** — if the user supplied `--from <path>` (or a trailing positional path), the slash command reads the file. Supported formats: Markdown (free-form review text), JSON (already-structured comments matching tai's payload schema, partially or fully enriched), or any plain-text variant the user names. The slash command auto-detects format by extension and content.
   - **Current conversation context** — if no file path is given, the slash command looks for review content already present in the current Claude conversation (e.g. a prior AI's review output that surfaced in the same session, or content the user pastes in response to the slash command's prompt).
   - **Prompt the user** — if neither of the above yields content, the slash command asks the user where the review is and pauses for input (pasted text, a path, or a description).

   In all three sub-paths the slash command MUST:
   - Parse the content into discrete comments. For each, extract `file`, `lines` (best-effort; ranges or single-line accepted), and a draft `title`/`description`.
   - Enrich each comment with the five mandatory fields per step 4 below — even if the upstream source already provided some of them, the slash command MUST confirm each field with the user before piping. The one exception: when the source is a JSON file already conforming to tai's payload schema, the slash command MAY skip per-field confirmation and surface a single confirmation summary instead.
   - The slash command MUST NOT call `gh` or `st_reviews` in this mode.

   Manual mode is the right path whenever the review wasn't pulled from a GitHub PR. Typical sources include: an AI run against staged changes, a teammate's review pasted from chat, a linter or static-analysis tool's output written to a file, an audit report.
4. **Enrichment**. For each parsed comment, the slash command MUST produce the five mandatory fields (`severity`, `category`, `why_fix`, `suggested_fix`, `consequences`). It MUST show each draft to the user before piping it to the CLI and MUST NOT silently invent enrichment.
5. **External refs by mode**. Each comment's `external_refs[]` array carries provenance the CLI uses for idempotent re-import:
   - Remote mode: refs use GitHub-derived kinds (`github-pr-comment`, `github-review-body`, `github-issue-comment`) with the vendor numeric IDs as `id`.
   - Manual mode: refs use `kind = "manual"`. The `id` is derived deterministically from the comment's content (sha256 of `file + lines + title`, truncated). Deterministic IDs make re-running `/tai:import branch <name>` for the same review idempotent without duplicating rows. When the source is a JSON file that already supplies external_refs, the slash command MAY pass those refs through verbatim.
6. **Deduplication**. In remote mode, multiple reviewers flagging the same issue MUST be combined into one comment with all relevant external_refs attached and a `source` display string like `"coderabbit + greptile"`. In manual mode, deduplication is the user's responsibility — the slash command may surface near-duplicates for confirmation but MUST NOT silently merge them.
7. **Batching**. Groups of comments sharing a corrective action MUST be assigned a `batch_key` (`B1`, `B2`, …); the batch row appears in the payload's `batches[]`.
8. **Invocation**. The slash command pipes the assembled JSON payload to `tai import -` via shell redirection. It MUST surface the CLI's summary back to the user verbatim.
9. **Stack mode loop**. For `stack` scope, the slash command produces one payload per PR and pipes each to a separate `tai import -` invocation. It MUST process PRs ancestor-first. Stack mode only applies to remote collection; there is no "stack" form of context mode.
10. **Error handling**. On `IMPORT_AMBIGUOUS_REFS`, the slash command MUST surface the conflicting comment IDs and ask the user how to reconcile (typically by omitting one of the ambiguous refs from the payload and re-running).

The slash command MUST NOT bypass the CLI by writing to the database directly.

#### Scenario: Remote mode requires gh or staccato

- **WHEN** the user invokes `/tai:import` and neither `gh` nor staccato MCP is available
- **THEN** the slash command exits without invoking `tai import` and tells the user how to enable a collection backend

#### Scenario: Stack mode loops per PR

- **WHEN** the user invokes `/tai:import stack` against a stack of three PRs
- **THEN** the slash command invokes `tai import -` exactly three times, in ancestor-first order

#### Scenario: Manual mode from conversation context

- **WHEN** the user invokes `/tai:import branch feat/x` with no `--from` path
- **AND** the current conversation contains review output from a prior AI invocation
- **THEN** the slash command parses the review content from conversation context
- **AND** does not invoke `gh` or `st_reviews`
- **AND** pipes the assembled payload to `tai import -` with `target.kind = "branch"` and `target.branch.name = "feat/x"`

#### Scenario: Manual mode from a markdown file

- **WHEN** the user invokes `/tai:import branch feat/x --from ./review.md`
- **THEN** the slash command reads `./review.md`, parses comments from its markdown body, enriches with user confirmation, and pipes the payload to `tai import -`

#### Scenario: Manual mode from a JSON file already conforming to the payload schema

- **WHEN** the user invokes `/tai:import branch feat/x --from ./review.json`
- **AND** `review.json` already matches tai's import payload schema
- **THEN** the slash command MAY skip per-field confirmation and surface a single summary instead
- **AND** pipes the file's contents (with `target` patched to the requested branch) to `tai import -`

#### Scenario: Manual mode prompts when source is unclear

- **WHEN** the user invokes `/tai:import branch feat/x`
- **AND** no `--from` path is supplied
- **AND** the conversation contains no recognisable review content
- **THEN** the slash command asks the user where the review is (paste, path, or description)
- **AND** waits for input before proceeding

#### Scenario: Manual mode produces deterministic external refs

- **WHEN** the same review (same file content) is imported twice in succession via `/tai:import branch feat/x --from ./review.md`
- **THEN** the second invocation's `external_refs[].id` values match the first invocation's
- **AND** the CLI's re-import upsert preserves the original `comments.id` rows (refs match, comments are not duplicated)

#### Scenario: Manual mode never calls gh or st_reviews

- **WHEN** the user invokes `/tai:import branch feat/x` in any sub-path (file / context / prompt)
- **THEN** the slash command body does not invoke `gh api`, `gh pr`, or `st_reviews` MCP tools during this invocation
