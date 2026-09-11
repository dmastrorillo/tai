## ADDED Requirements

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
