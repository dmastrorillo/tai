# update-banner Specification

## Purpose
TBD - created by archiving change pivot-to-ai-as-code. Update Purpose after archive.
## Requirements
### Requirement: Background update check

On every TAI invocation, the system SHALL evaluate the cache file at `<TAI_DATA_DIR>/state/update-check.json`. If the cache is missing, malformed, or older than the configured `update-check-interval` (default `6h`), TAI SHALL spawn a non-blocking background goroutine that:

1. Queries the latest TAI release from a hard-coded source URL.
2. For each installed plugin, queries the latest release from the plugin's recorded source.
3. For the configured source repo, queries the upstream tip of the default branch.
4. Writes the consolidated result to `<TAI_DATA_DIR>/state/update-check.json` with a fresh timestamp.

If `update-check-interval` is `0`, the background check SHALL NOT run.

The background check MUST NOT block the foreground command. Failures (network unreachable, 5xx, rate limit) are absorbed silently; the cache file is not updated and the next invocation will retry per the cadence rule.

#### Scenario: Check fires when cache is stale

- **WHEN** the cache file's timestamp is older than `update-check-interval` and TAI is invoked
- **THEN** the foreground command completes without blocking on the poll
- **AND** within a short bounded wait after exit, `<TAI_DATA_DIR>/state/update-check.json` has a timestamp newer than the stale timestamp

#### Scenario: Check skipped when cache is fresh

- **WHEN** the cache file's timestamp is within `update-check-interval`
- **AND** TAI is invoked
- **THEN** `<TAI_DATA_DIR>/state/update-check.json` is byte-identical before and after the command exits

#### Scenario: Disabled via interval=0

- **WHEN** `update-check-interval: 0` is configured and the cache file is stale
- **AND** TAI is invoked
- **THEN** `<TAI_DATA_DIR>/state/update-check.json` is byte-identical before and after the command exits

### Requirement: Once-per-day aggregated banner

When the cache file shows at least one pending update (TAI itself, an installed plugin, or the source-repo branch tip) AND the cache's `last-banner-date` field does not equal today's date (in the user's local time zone), the system SHALL print an aggregated banner to stderr on the current command, then update the `last-banner-date` to today.

The banner MUST:

- Be on stderr only.
- Be prefixed with the literal token `[tai]` on every line so AI agents can recognize and segment it.
- Be at most 4 short lines.
- Name every pending update with its current → available version (or, for the source repo, a count of new commits).
- Name the exact command the user runs to update each (e.g., `tai plugins update triage`).
- For TAI itself, name a representative package-manager command (`brew upgrade tai`, `go install ...@latest`) — TAI does not perform self-updates.

#### Scenario: Banner fires on first command of the day

- **WHEN** the cache file shows TAI 1.3.0 available (current 1.2.0) and `last-banner-date` is yesterday
- **AND** the user runs any TAI command
- **THEN** stderr contains a `[tai]` banner naming the upgrade
- **AND** the cache's `last-banner-date` is updated to today

#### Scenario: Banner suppressed on subsequent commands the same day

- **WHEN** the banner has already fired today and the user runs another command
- **THEN** no banner is printed on stderr

#### Scenario: No banner when nothing pending

- **WHEN** the cache file shows no pending updates
- **THEN** no banner is printed regardless of `last-banner-date`

### Requirement: TAI does not self-update

The CLI SHALL NOT include a verb that updates the `tai` binary in place. The banner SHALL name the package-manager command (e.g., `brew upgrade tai`) but SHALL NOT execute it.

#### Scenario: No self-update verb

- **WHEN** the user runs `tai update`
- **THEN** the command exits with `UNKNOWN_SUBCOMMAND`
- **AND** the "what to do" bullets explain that TAI is updated via the user's package manager

### Requirement: Prefix-aware plugin lookup in background update check

When the background update-check goroutine queries the latest release for each installed plugin (step 2 of the existing `Background update check` requirement), it SHALL use the prefix-aware lookup algorithm defined in the `release-cycle` capability — NOT the GitHub `/releases/latest` endpoint.

For each installed plugin recorded in `<TAI_DATA_DIR>/state/plugins.json`:

1. Read the plugin's recorded `source.host`, `source.repo`, and `name`.
2. Determine the plugin's tag prefix. For first-party plugins served from this monorepo (recognized by the registry entry's `Source.Repo` matching the monorepo), the prefix is `plugins/<name>/`. For third-party plugins, the prefix is empty (match every tag).
3. Apply the algorithm: `GET /repos/{repo}/releases?per_page=100`, filter by prefix, drop pre-releases, parse the prefix-stripped suffix as semver, return max.
4. Write the resolved version (or "no release" sentinel) into the consolidated cache file.

The TAI-core lookup (step 1 of the existing `Background update check` requirement) SHALL continue to use the GitHub `/releases/latest` endpoint. Core tags carry no prefix and the endpoint already excludes pre-releases.

#### Scenario: Banner plugin-row reflects prefix-aware latest

- **GIVEN** triage `v0.4.0` is installed and `dmastrorillo/tai` has releases `v0.6.1` (core, newest), `plugins/triage/v0.5.0`, `v0.6.0` (core), `plugins/triage/v0.4.0`
- **WHEN** the background update check runs to completion
- **AND** the next day's first command fires the banner
- **THEN** the banner contains a row for `triage 0.4.0 → 0.5.0`
- **AND** does NOT report `triage 0.4.0 → 0.6.1` (the core release is NOT cross-contaminated into the triage row)

#### Scenario: Banner plugin-row skips pre-release

- **GIVEN** triage `v0.4.0` is installed and the most recent triage release is `plugins/triage/v0.5.0-rc.1` (pre-release), with no `plugins/triage/v0.5.0` stable
- **WHEN** the background update check runs to completion
- **THEN** the cache file records no available triage update
- **AND** no `triage` row appears in the banner

#### Scenario: Banner core-row continues using /releases/latest

- **GIVEN** the source repo has core releases `v0.6.1`, `v0.6.0`, and a pre-release `v0.7.0-rc.1`
- **AND** installed core is `v0.6.0`
- **WHEN** the background update check runs to completion
- **THEN** the cache file records `tai 0.6.0 → 0.6.1` (not 0.7.0-rc.1)
- **AND** the banner reflects the same

### Requirement: First-run onboarding hint

On the FIRST `tai` invocation for which no marker file at `<TAI_DATA_DIR>/state/first-run.json` exists, the system SHALL emit a single onboarding hint to stderr after the foreground command completes and BEFORE the process exits. The hint text is exactly:

```
→ Get started: run `tai install-commands` to make tai's commands available in your AI tool.
```

The hint is AI-tool-agnostic — it never names a specific AI tool (Claude, Cursor, Cody, etc.).

After printing the hint, the system SHALL best-effort write the marker file with shape `{"first-run": "<ISO-8601 UTC timestamp>"}`. The marker's existence is what matters; the timestamp is informational only. If the write fails (HOME unwritable, permission denied, disk full), the hint MAY print again on the next invocation — this is an acceptable failure mode.

The first-run hint and the once-per-day update banner are mutually exclusive on the same invocation. If both would fire (the very first run also has a pending update flagged in the cache), the system SHALL print only the first-run hint and SHALL update the banner's `last-banner-date` to today, so the update banner becomes eligible to fire the NEXT day instead of stacking with the first-run hint.

`tai install-commands` itself does NOT need to have run for the marker to be written; running any tai verb (including `tai --version`, `tai --help`, or a no-op invocation) creates the marker.

The hint is suppressed when:

- The marker file already exists.
- `tai` is invoked with no arguments AND the user is in a non-TTY context (CI scripts that probe `tai --version` shouldn't be flooded with prompts).

#### Scenario: First invocation prints the hint and writes the marker

- **GIVEN** `<TAI_DATA_DIR>/state/first-run.json` does NOT exist
- **WHEN** the user runs any TAI command (e.g. `tai --version`)
- **THEN** stderr contains the literal line `→ Get started: run \`tai install-commands\` to make tai's commands available in your AI tool.`
- **AND** `<TAI_DATA_DIR>/state/first-run.json` exists after the command completes
- **AND** the file contains a JSON object with a `first-run` field set to an ISO-8601 UTC timestamp

#### Scenario: Subsequent invocations suppress the hint

- **GIVEN** `<TAI_DATA_DIR>/state/first-run.json` exists
- **WHEN** the user runs any TAI command
- **THEN** the first-run hint is NOT printed
- **AND** the marker file's timestamp is unchanged

#### Scenario: First run with pending update prefers the hint

- **GIVEN** `<TAI_DATA_DIR>/state/first-run.json` does NOT exist
- **AND** `<TAI_DATA_DIR>/state/update-check.json` records a pending TAI upgrade with `last-banner-date` set to yesterday
- **WHEN** the user runs any TAI command
- **THEN** stderr contains the first-run hint
- **AND** the daily update banner is NOT printed on this invocation
- **AND** `last-banner-date` is updated to today (so the banner is eligible to fire on the next day, not stacked here)

#### Scenario: Marker write failure does not abort the command

- **GIVEN** `<TAI_DATA_DIR>/state/` is non-writable (permissions error)
- **WHEN** the user runs any TAI command
- **THEN** the foreground command completes with its expected exit code
- **AND** the first-run hint is printed (best-effort)
- **AND** no marker file is created
- **AND** the next invocation MAY print the hint again
