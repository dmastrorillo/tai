## ADDED Requirements

### Requirement: Source-repo clone location

The system SHALL maintain exactly one git clone of the configured source repo, in TAI's data directory at `<TAI_DATA_DIR>/source/`. The location is not configurable. The clone MUST NOT live under any configured target. On first `tai sync` after `repo-url` is set, the clone is created via `git clone`. On subsequent syncs, the existing clone is reused.

#### Scenario: First sync creates the clone

- **WHEN** the user runs `tai sync` for the first time with a configured `repo-url`
- **THEN** `<TAI_DATA_DIR>/source/.git` exists after the command completes

#### Scenario: Subsequent sync reuses the clone

- **WHEN** the user runs `tai sync` a second time
- **THEN** the existing clone is updated, not re-cloned

### Requirement: Eager fetch with cache fallback

`tai sync` SHALL invoke `git fetch` (and update the local branch tracking `origin/<default-branch>`) before reading the clone. If the fetch fails (network unreachable, authentication error, etc.), TAI MUST print a one-line warning to stderr naming the failure category and the timestamp of the last successful fetch, then proceed with the read against the cached state. The exit code is determined by the sync operation itself, not by the fetch outcome.

A `--offline` flag SHALL NOT exist; the implicit fallback covers the offline case.

#### Scenario: Network fetch fails

- **WHEN** `tai sync` runs while the network is unavailable
- **THEN** stderr contains a one-line warning naming "fetch failed" and the last-success timestamp
- **AND** the sync proceeds against the cached clone

#### Scenario: Fetch succeeds

- **WHEN** `tai sync` runs and the fetch succeeds
- **THEN** the clone is fast-forwarded to the upstream's tip before the read
- **AND** no fetch-failure warning is printed

### Requirement: M1 overwrite detection — existence-only

For each target file path TAI would write, the system SHALL determine "would overwrite" by checking only whether a file exists at the destination — no byte-level comparison is performed. Existing destination paths are batched into a single overwrite list across all configured targets and surfaced in one prompt. The destination paths grouped under the categories `skills`, `commands`, `agents`.

The prompt MUST be the only interactive element of `tai sync`. Other progress (file counts, fetch status) goes on stderr without blocking.

#### Scenario: Fresh sync to empty target

- **WHEN** the source repo has 3 skills and the target has no existing files
- **THEN** the sync writes all 3 files
- **AND** no overwrite prompt is shown

#### Scenario: Sync with one overwrite

- **WHEN** the source repo has a skill `foo` and the target already has a file at `<target>/<skills>/foo`
- **THEN** TAI prompts on stderr listing `foo` under skills as "will be overwritten"
- **AND** waits for `y` or `N` on stdin

#### Scenario: Sync with multiple overwrites is batched

- **WHEN** the source has 5 skills, 2 commands, and 1 agent that all exist at their target destinations
- **THEN** TAI emits one prompt grouping the 8 paths under their three categories
- **AND** does not prompt individually per file

### Requirement: `-y` flag bypasses the overwrite prompt

The system SHALL accept `-y` (and `--yes` as a long-form alias) on `tai sync`. When present, the overwrite prompt is suppressed and TAI proceeds as if the user answered `y`. The flag MUST work with `--prune` and with the no-prune case.

#### Scenario: -y skips the prompt

- **WHEN** the user runs `tai sync -y` with overwrites pending
- **THEN** TAI proceeds without prompting
- **AND** the overwritten files are listed on stderr after writing for visibility

#### Scenario: User rejects the prompt

- **WHEN** the user runs `tai sync` (no `-y`), TAI prompts, and the user answers `N`
- **THEN** TAI exits 0 without writing any files
- **AND** stderr contains a message naming the cancellation

### Requirement: Per-target manifest tracks installed paths

The system SHALL maintain one manifest file per configured target at `<TAI_DATA_DIR>/manifests/<sha256-of-target-root>.json`. The manifest is a JSON object listing every relative path TAI has installed into that target and not yet pruned. Each successful `tai sync` appends new paths from the current source to the manifest; the manifest entries are never removed except by `tai sync --prune`.

The manifest MUST NOT live inside any target directory.

#### Scenario: First sync creates the manifest

- **WHEN** the first `tai sync` writes 3 skills, 1 command, 0 agents to a target
- **THEN** a manifest file exists at the per-target path with 4 entries

#### Scenario: Subsequent sync extends the manifest

- **WHEN** a second `tai sync` writes 1 new skill (4 total in source now)
- **THEN** the manifest contains the union of the first sync's 4 paths and the new path (5 entries)

### Requirement: `tai sync --prune` deletes orphans

The system SHALL accept `--prune` on `tai sync`. When present, after writing source files, TAI computes the orphan set as `(manifest_entries) - (current_source_paths)` and deletes those paths from each configured target. Orphans are surfaced in the same batched prompt that lists overwrites (or in the no-prompt path under `-y`).

Without `--prune`, orphans persist in the target and the manifest. The sync summary on stderr MUST surface a line like `N orphans pending — run \`tai sync --prune\` to delete` whenever the orphan count is greater than 0.

#### Scenario: Prune deletes a removed source file

- **WHEN** a skill was synced previously, the source repo has removed it, and the user runs `tai sync --prune` accepting the prompt
- **THEN** the file no longer exists in the target
- **AND** the manifest no longer contains the path

#### Scenario: Sync without prune surfaces orphan count

- **WHEN** a previously-synced file has been removed from source and the user runs `tai sync` without `--prune`
- **THEN** the sync completes without deleting the file
- **AND** the stderr summary lists `1 orphan pending`

### Requirement: Background fetch poll surfaces source-repo updates

The system SHALL run a non-blocking background goroutine on every TAI invocation that, when the cached update-check is stale (older than `update-check-interval`), performs a remote-ref check against the source repo's default branch and records the result into `<TAI_DATA_DIR>/state/update-check.json`. The recorded state is consumed by the update-banner capability.

The background check MUST NOT delay command completion. If it errors (network unreachable, auth failure, 5xx), the failure is silently absorbed: the cache file is not modified, no error is written to stdout or stderr, and the next invocation retries per the cadence rule.

#### Scenario: Stale-cache poll refreshes the state file

- **WHEN** `<TAI_DATA_DIR>/state/update-check.json` has a timestamp older than `update-check-interval` and the configured source repo is reachable
- **AND** the user runs any TAI command
- **THEN** the foreground command completes without blocking on the poll
- **AND** within a short bounded wait after exit, `<TAI_DATA_DIR>/state/update-check.json` has a timestamp newer than the stale timestamp

#### Scenario: Fresh-cache poll does not touch the state file

- **WHEN** the cache file's timestamp is within `update-check-interval`
- **AND** the user runs any TAI command
- **THEN** `<TAI_DATA_DIR>/state/update-check.json` is byte-identical before and after the command exits

#### Scenario: Poll error is silently absorbed

- **WHEN** the cache file is stale and the configured source repo is unreachable
- **AND** the user runs any TAI command
- **THEN** the foreground command's stdout and stderr contain no error or warning attributable to the background poll
- **AND** `<TAI_DATA_DIR>/state/update-check.json` is byte-identical before and after the command exits

### Requirement: Background fetch is skipped when polling is disabled

When `update-check-interval` is configured to `0`, the background poll goroutine SHALL NOT run. The cache file is left untouched regardless of its age.

#### Scenario: Disabled poll leaves cache untouched

- **WHEN** `update-check-interval: 0` is configured and the cache file is stale
- **AND** the user runs any TAI command
- **THEN** `<TAI_DATA_DIR>/state/update-check.json` is byte-identical before and after the command exits
