---
name: "TAI: Import review"
description: "Pull review comments from GitHub or a manual source, enrich them, and import into tai."
category: "Workflow"
tags: [tai, import, review]
version: 1
---
# /tai-triage:import — capture review comments into tai's database

You are running as Claude inside Anthropic's Claude Code (or an
equivalent agent harness). This slash-command body is your instructions
for one invocation of `/tai-triage:import`.

Your job: turn an outside-world review (GitHub PR comments, an AI
review, a teammate's pasted notes, a linter report) into a strict JSON
payload, then pipe it to `tai triage import -`. The CLI validates the payload
and persists it to the user's local SQLite database. The CLI does not
talk to GitHub, does not read files, and does not enrich anything — all
of that is YOUR job.

You MUST follow this contract. Anything not specified here is left to
your judgement, but never deviate from the obligations below.

## 1. Resolve scope and collection mode

Each invocation maps to a scope AND a collection mode based on the
arguments the user passed:

| Invocation                                          | Scope                                    | Collection mode |
|-----------------------------------------------------|------------------------------------------|-----------------|
| `/tai-triage:import`                                       | current PR                               | remote          |
| `/tai-triage:import <pr-number-or-url>`                    | that PR                                  | remote          |
| `/tai-triage:import stack`                                 | every PR trunk → current, ancestor first | remote          |
| `/tai-triage:import branch <name> [--from <path>]`         | that branch                              | manual          |

The collection mode determines where the raw review content comes from:

- **Remote** — pull live from GitHub or staccato. Use this for PR work.
- **Manual** — source the review from a file, the conversation, or by
  asking the user. Use this for branch work where the review wasn't
  pulled from a GitHub PR.

You MUST NOT mix modes inside one invocation. A `branch` invocation
NEVER calls `gh` or `st_reviews`. A PR invocation NEVER tries to read a
file or pull from conversation context.

## 2. Remote collection mode (PR / stack scopes)

Preferred backend: staccato MCP's `st_reviews`. Call
`st_reviews(scope='current')` for a single PR, or
`st_reviews(scope='to-current')` for stack mode.

Fallback backend: the `gh` CLI. For a PR `<N>` in `<owner>/<name>`:

- `gh api repos/<owner>/<name>/pulls/<N>/comments`
- `gh api repos/<owner>/<name>/pulls/<N>/reviews`
- `gh api repos/<owner>/<name>/issues/<N>/comments`

If neither backend is available (no `gh` on PATH, no staccato MCP
connected), STOP. Tell the user which to install — `gh auth login` for
GitHub CLI, or "connect the staccato MCP server" — and exit without
invoking `tai triage import`.

For stack mode, repeat the per-PR flow for each PR ancestor-first; each
PR becomes a separate `tai triage import -` invocation. Stack mode is the
slash command's loop; the CLI never sees a multi-PR payload.

## 3. Manual collection mode (branch scope)

Sources are tried in order. Use the first one that yields review
content. Do NOT try the next source after one succeeds.

1. **File path** — when the user supplied `--from <path>` (or a
   trailing positional path that resolves to an existing file), read
   that file.
   - Markdown / plain text: parse as free-form review notes.
   - JSON: try to parse as a tai payload (matching the schema below).
     If it matches, the file is the payload; you MAY skip per-field
     confirmation and surface a single summary instead. If the JSON
     supplies `external_refs`, pass them through verbatim.
   - Anything else: best-effort plain-text parse.
2. **Current conversation context** — when no path is given, look in
   the current Claude conversation for review output (e.g. a prior AI
   review that surfaced in this session). If you find it, work with it.
3. **Prompt the user** — when neither yields content, ask the user
   where the review is. Wait for input before continuing.

You MUST NOT call `gh`, `gh api`, `gh pr`, or any `st_reviews` MCP tool
in manual mode. Manual mode never touches the network.

## 4. Enrichment (every comment, both modes)

Every comment you produce MUST carry these five fields. None is
optional, all five are required by the CLI's validator:

- `severity` — one of `critical`, `major`, `minor`, `nitpick`.
- `category` — one of `security`, `correctness`, `feature-regression`,
  `code-quality`, `performance`, `testing`.
- `why_fix` — one sentence: why this matters.
- `suggested_fix` — concrete steps the author can take.
- `consequences` — what happens if the issue is left unfixed.

Show each draft to the user before piping. Confirm each field
explicitly (or one consolidated confirmation if the source is already a
tai-shaped JSON file). NEVER silently invent enrichment — if you don't
know, ASK the user.

If you have access to the file under review, read it and the
surrounding code BEFORE drafting enrichment so the recommendations are
grounded in the actual change.

## 5. External refs (provenance for idempotent re-import)

Every comment carries an `external_refs[]` array with at least one
entry. The shape of each ref depends on the collection mode:

- **Remote mode** — use GitHub-derived kinds:
  - `github-pr-comment` for inline PR review comments. `id` is the
    GitHub comment numeric ID as a string.
  - `github-review-body` for the body of a review (no inline anchor).
    `id` is the GitHub review numeric ID.
  - `github-issue-comment` for top-level issue/PR comments. `id` is
    the GitHub comment numeric ID.
  - Set `reviewer` to the GitHub login when available (`coderabbit`,
    `greptile`, the human reviewer's username, etc.).
- **Manual mode** — use `kind = "manual"`. The `id` is a deterministic
  hash of the comment content: SHA-256 of `file + lines + title`,
  truncated to the first 16 hex chars. Deterministic IDs make
  re-running `/tai-triage:import branch <name> --from ./review.md` idempotent
  — the CLI sees the same refs and updates the existing comment
  instead of duplicating. When the source JSON already supplies
  refs, pass them through verbatim instead of recomputing.

## 6. Deduplication and batching

- **Remote mode**: when multiple reviewers flag the same issue (same
  file + roughly the same lines + same intent), combine them into ONE
  comment with all relevant external_refs attached and a `source`
  display string like `"coderabbit + greptile"`.
- **Manual mode**: deduplication is the user's responsibility. Surface
  obvious near-duplicates for confirmation but do NOT silently merge
  them.

Group comments that share a corrective action into a batch. Assign each
batch a key (`B1`, `B2`, …) and add a row to the payload's `batches[]`
array with a short title summarising the shared action.

## 7. Payload shape

Produce JSON matching this shape exactly. The CLI rejects unknown
fields, so don't add anything not listed here.

```json
{
  "repo": "<owner>/<name>",
  "target": {
    "kind": "pr",
    "pr": {
      "number": 142,
      "title": "feat: oauth",
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
      "description": "execSync interpolates user input into a shell string. Use execFileSync.",
      "why_fix": "execSync runs through a shell, so values containing shell metacharacters are interpreted.",
      "suggested_fix": "Replace execSync(`git config ${k} ${v}`) with execFileSync('git', ['config', k, v]).",
      "consequences": "An attacker who can influence k or v gains arbitrary command execution in the build environment.",
      "batch_key": "B1"
    }
  ]
}
```

For a branch scope, swap the `target` body for:

```json
"target": {
  "kind": "branch",
  "branch": { "name": "feat/oauth" }
}
```

## 8. Invocation

Pipe the assembled JSON to `tai triage import -` via shell redirection. Use a
heredoc or temp file — do NOT pass the payload as an argument (argv
limits would bite on large reviews).

Example (single PR):

```sh
cat <<'EOF' | tai triage import -
{ "repo": "acme/app", ... }
EOF
```

Surface the CLI's success summary back to the user verbatim. The
summary tells them how many comments were inserted vs. updated vs.
frozen.

## 9. Error handling

If `tai triage import` exits non-zero:

- `IMPORT_INVALID_JSON` — your JSON is malformed. Look at the stderr
  message, fix the payload, and re-run.
- `IMPORT_SCHEMA_INVALID` — one or more fields are missing or invalid.
  The stderr lists every violation with a JSON-pointer-style path. Fix
  every line before re-running.
- `IMPORT_AMBIGUOUS_REFS` — one of the comments has external_refs that
  point to two different existing comment rows. The stderr lists the
  conflicting comment IDs. Surface them to the user and ask which one
  to keep. Typically the right resolution is to drop one of the
  ambiguous refs from the payload and re-run.
- Other errors (storage / data dir) — show the stderr message to the
  user. They will need to address it before re-running.

## 10. Things you MUST NOT do

- Do NOT bypass the CLI by writing to the SQLite database directly.
- Do NOT invent enrichment to silence the user. Ask if you don't know.
- Do NOT silently merge comments in manual mode. Confirm.
- Do NOT call `gh` or `st_reviews` in manual (branch) mode.
- Do NOT skip enrichment fields and hope the CLI accepts them — every
  enrichment field is required.
- Do NOT mix multiple PRs into one `tai triage import` invocation; stack mode
  loops once per PR.
