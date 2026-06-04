## ADDED Requirements

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
