## ADDED Requirements

### Requirement: Bundled slash command exists and obeys the command-framework

The system SHALL bundle a slash command at `commands/verify.md` whose frontmatter conforms to the `command-framework` schema and whose body is installed at `~/.claude/commands/tai/verify.md` by `tai install`.

The frontmatter MUST include:

| Field | Value |
|---|---|
| `name` | `"TAI: Verify"` |
| `description` | `"Check whether accepted comments have been addressed; mark completed."` |
| `category` | `"Workflow"` |
| `tags` | `[tai, triage, verify]` |
| `version` | `1` |
| `content_hash` | `sha256:<hex>` computed over the body |

A corresponding `commands/verify.ledger.json` MUST exist with at least one entry — the current `content_hash`.

#### Scenario: Slash command bundled and installable

- **WHEN** `tai install` is invoked after this proposal lands
- **THEN** a file is written at `~/.claude/commands/tai/verify.md`
- **AND** that file's frontmatter contains `name: "TAI: Verify"`, `version: 1`, and a valid `content_hash`

### Requirement: Invocation forms

The slash command SHALL accept exactly these invocation forms:

- `/tai:verify` — auto-detect current scope.
- `/tai:verify --pr <number-or-url>` — single PR by number or full URL.
- `/tai:verify --branch <name>` — branch-scoped review.
- `/tai:verify stack` — every PR from trunk to the current branch, ancestor-first.

Any other argument shape MUST surface a usage error in the conversation, listing the four supported forms.

#### Scenario: stack mode requires gh or staccato

- **WHEN** the user invokes `/tai:verify stack` and neither `gh` nor staccato MCP is available
- **THEN** the slash command body announces the missing dependency and exits without entering the loop

### Requirement: Scope resolution via the CLI

The slash command body SHALL resolve operating scope by invoking `tai status` (with any user-supplied `--pr` / `--branch` flag passed through). The slash command MUST NOT implement its own scope detection.

If `tai status` returns `TRIAGE_NO_SCOPE` or `TRIAGE_AMBIGUOUS_SCOPE`, the slash command MUST handle the failures identically to `/tai:triage`: surface guidance, exit the conversation without entering the loop.

If the scope resolves but `tai list --status accepted` returns no comments, the slash command MUST announce "no accepted comments to verify in this scope" and exit without entering the loop.

#### Scenario: No accepted comments in scope

- **WHEN** the user invokes `/tai:verify` and the scope has no comments with status `accepted`
- **THEN** the slash command announces the empty queue
- **AND** does not invoke any further `tai` verbs beyond the initial `tai status` / `tai list --status accepted`

### Requirement: PR-scope evidence uses `gh pr diff`

For PR-target scopes, the slash command SHALL fetch the PR's unified diff via `gh pr diff <number> --patch` (or `gh api repos/{o}/{n}/pulls/{n}` for the raw patch). The diff MUST be the primary evidence source.

For each accepted comment, the slash command SHALL search the diff for:

1. **Removal of the original code** — the snippet referenced by the comment's `description` or implied by `lines` appearing in a removed (`-`) line block at the comment's file.
2. **Addition of the suggested-fix pattern** — the pattern derived from `suggested_fix` appearing in an added (`+`) line block at the comment's file.

The working tree MUST be checked as a secondary evidence source to catch fixes that are staged or unpushed.

If `gh pr diff` fails (network, auth, no PR), the slash command MUST:

- Warn the user that PR-diff evidence is unavailable.
- Fall back to working-tree-only verification (the branch-scope flow).
- Cap evidence confidence at "medium" — high-confidence verdicts based on working-tree alone are downgraded.

#### Scenario: PR-scope verification uses gh pr diff

- **WHEN** the user invokes `/tai:verify --pr 142` and `gh` is authenticated
- **THEN** the slash command invokes `gh pr diff 142` (or equivalent `gh api` call)
- **AND** parses the returned unified diff for evidence per the heuristics above

#### Scenario: gh failure falls back to working tree

- **WHEN** `gh pr diff` fails for any reason
- **THEN** the slash command warns the user
- **AND** proceeds with working-tree evidence only
- **AND** any evidence summary's confidence MUST NOT exceed "medium"

### Requirement: Branch-scope evidence uses working tree and git log

For branch-target scopes, the slash command SHALL gather evidence from:

1. The file at the comment's `file:lines` position in the current working tree. Check whether the original code is still present and whether the suggested-fix pattern appears.
2. `git log --oneline -10 -- <file>` — check whether any recent commit subject contains keywords from the comment's `title`.

The slash command MUST NOT invoke `gh pr diff` for branch-target scopes (there's no PR to diff against).

#### Scenario: Branch-scope evidence from working tree

- **WHEN** the user invokes `/tai:verify --branch feat/x`
- **THEN** the slash command reads files from the working tree and runs `git log` for evidence
- **AND** does not invoke `gh pr diff`

### Requirement: Three-tier evidence confidence

For each accepted comment, the slash command SHALL classify evidence into one of three confidence levels:

- **High**: qualifies under EITHER (a) the file referenced by the comment is gone entirely, OR (b) the original code is absent at the comment's lines AND the suggested-fix pattern is present in the working tree, with one extra condition for PR-target scopes: when `gh pr diff` succeeded, the PR diff MUST also corroborate the change; when `gh pr diff` failed (the gh-failure fallback), HIGH is downgraded to MEDIUM regardless of how conclusive the working-tree match looks. For branch-target scopes, working-tree signals alone suffice.
- **Medium**: exactly one of (original code absent, suggested-fix pattern present), OR a relevant git log subject containing keywords from the comment's `title`.
- **None**: neither check matches and no relevant git history.

The slash command SHALL surface the confidence level explicitly in the evidence summary block presented to the user.

#### Scenario: High confidence requires both signals

- **WHEN** at a comment's file/lines, the original code is absent AND the suggested-fix pattern is present
- **AND** (PR scope) the PR diff also shows the change
- **THEN** the evidence summary is labelled `HIGH confidence`

#### Scenario: Medium confidence on partial evidence

- **WHEN** the original code is absent but no clear suggested-fix pattern is found
- **THEN** the evidence summary is labelled `MEDIUM confidence`

#### Scenario: No evidence on no match

- **WHEN** none of the heuristics match
- **THEN** the evidence summary is labelled `NONE`
- **AND** the slash command MUST NOT prompt for completion (see "Conservative bias" below)

### Requirement: Confirmation loop with conservative bias

For each accepted comment with HIGH or MEDIUM evidence, the slash command SHALL:

1. Surface the evidence summary (file, lines, original-code status, suggested-fix-pattern status, PR-diff status if applicable, confidence label).
2. Prompt the user. The default depends on confidence:
   - HIGH → prompt defaults to `Y` (`Mark as completed? [Y/n]`).
   - MEDIUM → prompt defaults to `n` (`Mark as completed? [y/N]`).
3. Process the user's response:
   - Yes → call `tai complete <id> --resolution "<one-line evidence summary>"`.
   - No → leave the comment as `accepted`. Move to the next.
   - "Show me again" / "re-show" → call `tai show <id>` and re-prompt with the same evidence summary.

For comments with NONE confidence, the slash command MUST NOT prompt for completion. It announces "no evidence found; comment remains accepted" and moves on.

#### Scenario: HIGH confidence defaults to Y

- **WHEN** a comment's evidence summary is HIGH
- **AND** the user presses return on the prompt (default)
- **THEN** the slash command invokes `tai complete <id> --resolution …`

#### Scenario: MEDIUM confidence defaults to n

- **WHEN** a comment's evidence summary is MEDIUM
- **AND** the user presses return on the prompt (default)
- **THEN** the slash command does NOT invoke `tai complete`
- **AND** moves to the next comment

#### Scenario: NONE confidence skips the prompt

- **WHEN** a comment's evidence summary is NONE
- **THEN** the slash command announces "no evidence found" without prompting
- **AND** does not invoke `tai complete`

#### Scenario: Show-again loops back

- **WHEN** the user replies "show me again" to a verify prompt
- **THEN** the slash command invokes `tai show <id>` and re-prompts

### Requirement: Recap at end of loop

When every accepted comment has been processed, the slash command SHALL emit a recap containing:

1. A header line naming the scope (e.g. `Verification complete for acme/app PR #142.`).
2. Counts:
   - `Marked completed: <N>`
   - `Left accepted:    <M>` (the residual queue size after this run)
3. The still-accepted work queue: every comment still in `accepted` status, sorted by severity then file, each row showing `[<sev-abbr>] <id>: <title> (<file>:<lines>)` where `<id>` is the integer position number `tai list` prints in its `ID` column.
4. A closing line suggesting the prune command, scope-qualified so the CLI accepts it: `Ready to prune the completed comments? Run \`tai forget <scope-flag> --status completed --yes\`.` where `<scope-flag>` is `--pr <N>` for PR-scope runs and `--branch <name>` for branch-scope runs. `tai forget` rejects invocations without a primary selector; `--status` alone is a filter, not a selector.

The recap MUST be emitted exactly once per loop completion.

For `stack` mode, the recap is per-PR with a final stack-level aggregate after the last PR.

#### Scenario: Recap names the still-accepted queue

- **WHEN** verification completes with 3 marked completed and 5 left accepted
- **THEN** the recap lists each of the 5 still-accepted comments in severity order

#### Scenario: Recap suggests the scope-qualified prune command

- **WHEN** verification completes with at least one comment marked completed
- **THEN** the recap's final line contains a scope-qualified prune command — `tai forget --pr <N> --status completed --yes` for PR-scope runs, or `tai forget --branch <name> --status completed --yes` for branch-scope runs
- **AND** the command MUST NOT omit the `--pr` / `--branch` selector (`tai forget` exits `TRIAGE_INVALID_FLAGS` without one)

#### Scenario: Stack mode recaps per PR and at the end

- **WHEN** `/tai:verify stack` completes for a stack of three PRs
- **THEN** three per-PR recaps appear in conversation
- **AND** a final stack-level aggregate count appears after the last PR's recap

### Requirement: Slash command persistence is via the CLI only

The slash command body SHALL NOT write to the database directly. State changes (only ever `tai complete`) MUST route through the CLI.

#### Scenario: All state changes route through tai complete

- **WHEN** the user confirms a completion during the verify loop
- **THEN** the resulting state change is observable as a `tai complete <id>` invocation
- **AND** no direct SQLite writes occur from the slash command
