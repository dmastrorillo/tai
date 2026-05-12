# Implementation Tasks

> **Prerequisite (out of pipeline):** `add-tai-foundation`, `add-storage-schema`, `add-install-command`, `add-import-command`, and `add-triage-state` are all implemented. In particular, `tai list --status accepted` and `tai show --all --status accepted` (introduced in `add-triage-state`) and `tai forget --status …` (also in `add-triage-state`) MUST be available.
>
> This proposal authors a Claude slash command. No new Go code beyond embedding the new ledger file.

## 1. Author the slash command body

- [ ] 1.1 Create `commands/verify.md` with frontmatter matching `specs/verify-command/spec.md`: `name: "TAI: Verify"`, `description`, `category: "Workflow"`, `tags: [tai, triage, verify]`, `version: 1`, placeholder `content_hash`.
- [ ] 1.2 Body section 1 — invocation forms and scope resolution (mirror `/tai:triage`'s precheck via `tai status`; handle `TRIAGE_NO_SCOPE`, `TRIAGE_AMBIGUOUS_SCOPE`, and the "no accepted comments" empty-loop case).
- [ ] 1.3 Body section 2 — evidence gathering for PR-scope targets: `gh pr diff <N> --patch` parsing, diff-block-by-file lookups, working-tree cross-reference.
- [ ] 1.4 Body section 3 — evidence gathering for branch-scope targets: working-tree reads + `git log --oneline -10 -- <file>`.
- [ ] 1.5 Body section 4 — gh-failure fallback: warn user, drop to working-tree-only, cap confidence at MEDIUM.
- [ ] 1.6 Body section 5 — three-tier confidence labelling (HIGH / MEDIUM / NONE) with the exact criteria.
- [ ] 1.7 Body section 6 — confirmation prompts: HIGH defaults to `Y`, MEDIUM defaults to `n`, NONE skips the prompt entirely.
- [ ] 1.8 Body section 7 — show-again loopback handling.
- [ ] 1.9 Body section 8 — recap template (header, counts, still-accepted work queue, prune suggestion).
- [ ] 1.10 Body section 9 — stack-mode loop (per-PR cycle, between-PR pause, stack aggregate).
- [ ] 1.11 Body section 10 — guardrails: persist only via `tai complete`; never bypass the CLI; conservative bias on weak evidence.

## 2. Seed the hash ledger

- [ ] 2.1 Run the `tai-ledger` helper against `commands/verify.md` to compute the body hash.
- [ ] 2.2 Create `commands/verify.ledger.json` containing the computed hash as its single (initial) entry.
- [ ] 2.3 Backfill the computed hash into the `content_hash` frontmatter field.
- [ ] 2.4 Verify the build-time test (introduced by `add-install-command`) passes: current body hash equals the last ledger entry.

## 3. Install pipeline smoke test

- [ ] 3.1 After `tai install` runs, verify `~/.claude/commands/tai/verify.md` exists alongside `import.md` and `triage.md`.
- [ ] 3.2 Re-run `tai install`; verify all three commands classify as `up-to-date`.
- [ ] 3.3 Modify `verify.md` by one byte; re-run `tai install`; verify it classifies as `user-modified` and prompts (or honours `TAI_ACCEPT_COMMAND_UPDATES`).

## 4. Manual end-to-end exercise

- [ ] 4.1 Set up a test repo with an imported PR. Triage some comments to `accepted`.
- [ ] 4.2 Make code changes that address some accepted comments (clearly, with the suggested fix) and partially address others.
- [ ] 4.3 Run `/tai:verify` for that PR.
- [ ] 4.4 Verify HIGH-confidence comments are surfaced with the right evidence and the prompt defaults to `Y`.
- [ ] 4.5 Verify MEDIUM-confidence comments default to `n`.
- [ ] 4.6 Verify NONE-confidence comments are NOT prompted and are reported in the recap as "no evidence found".
- [ ] 4.7 Confirm a few completions; verify `tai status` shows the reduced accepted count.
- [ ] 4.8 Run the suggested `tai forget --status completed --yes`; verify the completed comments are gone and the rest remain.

## 5. gh-failure fallback exercise

- [ ] 5.1 Run `/tai:verify --pr <N>` in an environment with `gh` un-authenticated (e.g. unset `GH_TOKEN`).
- [ ] 5.2 Verify the slash command warns the user and continues with working-tree-only evidence.
- [ ] 5.3 Verify no comment is labelled HIGH confidence in this run.

## 6. Stack-mode exercise

- [ ] 6.1 Set up a staccato stack of 2 PRs with imported and partially-fixed comments.
- [ ] 6.2 Run `/tai:verify stack`.
- [ ] 6.3 Verify per-PR recaps appear in conversation.
- [ ] 6.4 Verify the final stack-level aggregate.

## 7. Documentation and validation

- [ ] 7.1 Cross-reference the slash command body against `specs/verify-command/spec.md`: every requirement is reflected in the markdown.
- [ ] 7.2 `openspec validate add-verify-command` reports no errors.
