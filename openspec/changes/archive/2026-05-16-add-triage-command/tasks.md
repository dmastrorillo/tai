# Implementation Tasks

> **Prerequisite (out of pipeline):** `add-tai-foundation`, `add-storage-schema`, `add-install-command`, `add-import-command`, and `add-triage-state` are all implemented. The CLI verbs `tai status`, `tai show`, `tai accept`, `tai dismiss`, `tai complete`, and `tai forget` work standalone.
>
> This proposal authors a Claude slash command, not Go code. There is no `test-cases.md` BDD category for slash commands — testing is by manual exercise plus the build-time hash verification.

## 1. Author the slash command body

- [x] 1.1 Create `commands/triage.md` with frontmatter matching `specs/triage-command/spec.md`: `name`, `description`, `category: "Workflow"`, `tags: [tai, triage, review]`, `version: 1`, placeholder `content_hash` (filled by the helper).
- [x] 1.2 Body section 1 — invocation forms. Document the four supported forms and how to parse them.
- [x] 1.3 Body section 2 — scope resolution. Describe the `tai status` precheck and the three failure modes (`TRIAGE_NO_SCOPE`, `TRIAGE_AMBIGUOUS_SCOPE`, empty scope).
- [x] 1.4 Body section 3 — Phase 1 investigation. Enumerate the four evidence sources and the conservative bias. Show the example completion call shape.
- [x] 1.5 Body section 4 — Phase 2 triage loop. Describe ordering (batches first, severity-sorted), the per-item present/decide/persist cycle, and the progress-line emission.
- [x] 1.6 Body section 5 — dismissal-debate contract. Spell out the severity-calibrated rounds, the cognitive-bias checklist, and the "record the conclusion, not the opening line" rule.
- [x] 1.7 Body section 6 — batch-override convention. Show the example split, the confirmation step, and the `mixed` status outcome.
- [x] 1.8 Body section 7 — recap. Show the literal recap template.
- [x] 1.9 Body section 8 — stack mode loop. Per-PR cycle, between-PR pause, stack-level summary.
- [x] 1.10 Body section 9 — guardrails. "Persist only via `tai`; no direct DB writes; no network calls."

## 2. Seed the hash ledger

- [x] 2.1 Run the `tai-ledger` helper (from `add-install-command`) against `commands/triage.md` to compute the current body hash.
- [x] 2.2 Create `commands/triage.ledger.json` containing a JSON array with that hash as its single (initial) entry.
- [x] 2.3 Backfill the computed hash into the `content_hash` frontmatter field of `commands/triage.md`.
- [x] 2.4 Verify the build-time test (introduced by `add-install-command` task 1.4) passes: current body hash equals the last entry of the ledger.

## 3. Install pipeline smoke test

- [x] 3.1 After `tai install` runs, verify `~/.claude/commands/tai/triage.md` exists. (Automated via TC-INST-044 against a `t.TempDir()` target — same install pipeline, real bundle.)
- [x] 3.2 Re-run `tai install`; verify the slash command classifies as `up-to-date` (no rewrite). (TC-INST-044 asserts `installer.Classify` returns `up-to-date` immediately after install.)
- [x] 3.3 Modify `~/.claude/commands/tai/triage.md` by one byte; re-run `tai install`; verify it classifies as `user-modified` and prompts. (TC-INST-044 appends a byte and asserts the classifier flips to `user-modified`; the prompt wiring is covered exhaustively by TC-INST-020..029.)

## 4. Manual end-to-end exercise

> The underlying machinery for 4.1 is already automated — `/tai:import`
> piping into `tai import -` is covered by TC-IMP-* and the bundled
> install round-trip is covered by TC-INST-043. The remaining items
> (4.2–4.5) exercise conversational behaviours — the triage loop's
> ordering, the dismissal-debate cadence, the batch-override parse,
> and the stack-mode handoffs — which by nature require a live Claude
> harness against a real (or carefully mocked) PR and cannot be pinned
> from `go test`. Left deliberately unchecked; they are user-driven
> smoke tests to run after merge, not part of the merge gate.

- [ ] 4.1 Import comments for a real (or mocked) PR via `/tai:import`.
- [ ] 4.2 Run `/tai:triage` for that PR; walk through accept / dismiss / complete decisions; verify each one shows up via `tai status` and `tai list`.
- [ ] 4.3 Exercise a batch override mid-loop; verify the resulting batch status is `mixed` in `tai status`.
- [ ] 4.4 Exercise a dismissal on a `critical` comment; verify the slash command challenges with a concrete scenario before persisting.
- [ ] 4.5 Run `/tai:triage stack` against a staccato stack of two PRs (or simulate); verify per-PR recaps and the final aggregate.

## 5. Documentation and validation

- [x] 5.1 Cross-reference the slash command body against `specs/triage-command/spec.md` — for each requirement, the body MUST contain the corresponding language.
- [x] 5.2 `openspec validate add-triage-command` reports no errors.
- [x] 5.3 Verify the install-time classifier (covered by `specs/install/spec.md`) treats both `triage.md` and `import.md` consistently. The shared `runBundledInstallSmoke` helper drives the `missing → install → up-to-date` chain for both verbs from a single seam (called by TC-INST-043 and TC-INST-044); TC-INST-044 additionally asserts the `up-to-date → user-modified` flip on a one-byte hand-edit. The two TCs are deliberately not symmetric — only one needs to pin the post-install mutation path, and TC-INST-044 carries it.

> **Note:** This proposal does not change Go source other than what's needed to embed the new ledger file. Any required adjustments to the build pipeline (e.g. ensuring `commands/triage.md` and `commands/triage.ledger.json` are picked up by `//go:embed`) are inherited from `add-install-command` and do not need to be re-done here.
