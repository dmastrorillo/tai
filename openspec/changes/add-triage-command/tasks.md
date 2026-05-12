# Implementation Tasks

> **Prerequisite (out of pipeline):** `add-tai-foundation`, `add-storage-schema`, `add-install-command`, `add-import-command`, and `add-triage-state` are all implemented. The CLI verbs `tai status`, `tai show`, `tai accept`, `tai dismiss`, `tai complete`, and `tai forget` work standalone.
>
> This proposal authors a Claude slash command, not Go code. There is no `test-cases.md` BDD category for slash commands — testing is by manual exercise plus the build-time hash verification.

## 1. Author the slash command body

- [ ] 1.1 Create `commands/triage.md` with frontmatter matching `specs/triage-command/spec.md`: `name`, `description`, `category: "Workflow"`, `tags: [tai, triage, review]`, `version: 1`, placeholder `content_hash` (filled by the helper).
- [ ] 1.2 Body section 1 — invocation forms. Document the four supported forms and how to parse them.
- [ ] 1.3 Body section 2 — scope resolution. Describe the `tai status` precheck and the three failure modes (`TRIAGE_NO_SCOPE`, `TRIAGE_AMBIGUOUS_SCOPE`, empty scope).
- [ ] 1.4 Body section 3 — Phase 1 investigation. Enumerate the four evidence sources and the conservative bias. Show the example completion call shape.
- [ ] 1.5 Body section 4 — Phase 2 triage loop. Describe ordering (batches first, severity-sorted), the per-item present/decide/persist cycle, and the progress-line emission.
- [ ] 1.6 Body section 5 — dismissal-debate contract. Spell out the severity-calibrated rounds, the cognitive-bias checklist, and the "record the conclusion, not the opening line" rule.
- [ ] 1.7 Body section 6 — batch-override convention. Show the example split, the confirmation step, and the `mixed` status outcome.
- [ ] 1.8 Body section 7 — recap. Show the literal recap template.
- [ ] 1.9 Body section 8 — stack mode loop. Per-PR cycle, between-PR pause, stack-level summary.
- [ ] 1.10 Body section 9 — guardrails. "Persist only via `tai`; no direct DB writes; no network calls."

## 2. Seed the hash ledger

- [ ] 2.1 Run the `tai-ledger` helper (from `add-install-command`) against `commands/triage.md` to compute the current body hash.
- [ ] 2.2 Create `commands/triage.ledger.json` containing a JSON array with that hash as its single (initial) entry.
- [ ] 2.3 Backfill the computed hash into the `content_hash` frontmatter field of `commands/triage.md`.
- [ ] 2.4 Verify the build-time test (introduced by `add-install-command` task 1.4) passes: current body hash equals the last entry of the ledger.

## 3. Install pipeline smoke test

- [ ] 3.1 After `tai install` runs, verify `~/.claude/commands/tai/triage.md` exists.
- [ ] 3.2 Re-run `tai install`; verify the slash command classifies as `up-to-date` (no rewrite).
- [ ] 3.3 Modify `~/.claude/commands/tai/triage.md` by one byte; re-run `tai install`; verify it classifies as `user-modified` and prompts.

## 4. Manual end-to-end exercise

- [ ] 4.1 Import comments for a real (or mocked) PR via `/tai:import`.
- [ ] 4.2 Run `/tai:triage` for that PR; walk through accept / dismiss / complete decisions; verify each one shows up via `tai status` and `tai list`.
- [ ] 4.3 Exercise a batch override mid-loop; verify the resulting batch status is `mixed` in `tai status`.
- [ ] 4.4 Exercise a dismissal on a `critical` comment; verify the slash command challenges with a concrete scenario before persisting.
- [ ] 4.5 Run `/tai:triage stack` against a staccato stack of two PRs (or simulate); verify per-PR recaps and the final aggregate.

## 5. Documentation and validation

- [ ] 5.1 Cross-reference the slash command body against `specs/triage-command/spec.md` — for each requirement, the body MUST contain the corresponding language.
- [ ] 5.2 `openspec validate add-triage-command` reports no errors.
- [ ] 5.3 Verify the install-time classifier (covered by `specs/install/spec.md`) treats both `triage.md` and `import.md` consistently — both start as `missing` on a clean target, transition to `up-to-date` after install, and to `user-modified` on hand-edit.

> **Note:** This proposal does not change Go source other than what's needed to embed the new ledger file. Any required adjustments to the build pipeline (e.g. ensuring `commands/triage.md` and `commands/triage.ledger.json` are picked up by `//go:embed`) are inherited from `add-install-command` and do not need to be re-done here.
