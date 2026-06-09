## ADDED Requirements

### Requirement: Plugin tag prefix convention

A first-party plugin distributed from this monorepo SHALL use GitHub release tags of the form `plugins/<name>/vX.Y.Z`, where `<name>` matches the plugin's directory name under `plugins/`. A third-party plugin distributed from its own repository MAY use any tag convention its release pipeline produces, but MUST publish release assets whose filenames match `core/internal/plugins.AssetFilename(<name>, <os>, <arch>)` (i.e. `tai-plugin-<name>-<os>-<arch>.tar.gz`).

The plugin host SHALL NOT assume any specific tag prefix for third-party plugins; it discovers releases via the algorithm in the next requirement, which takes the prefix as a parameter and defaults to "no prefix" (empty string) for third-party sources unless the install command was given an explicit `--version`.

#### Scenario: First-party plugin tag

- **WHEN** the maintainer publishes the triage plugin
- **THEN** the tag is `plugins/triage/vX.Y.Z` (matches the plugin's directory name)
- **AND** the release asset is named `tai-plugin-triage-<os>-<arch>.tar.gz`

#### Scenario: Third-party plugin tag (illustrative)

- **WHEN** a third-party author publishes a plugin from `github.com/acme/tai-plugin-custom`
- **THEN** the tag MAY be any form they choose (e.g. `v1.2.0`)
- **AND** the release asset MUST be named `tai-plugin-custom-<os>-<arch>.tar.gz`

### Requirement: Latest plugin release resolution

When `tai plugins install <name>` or `tai plugins update <name>` is invoked WITHOUT an explicit `--version`, the plugin host SHALL resolve the latest available version using the prefix-aware lookup algorithm defined in the `release-cycle` capability:

1. `GET /repos/{org}/{repo}/releases?per_page=100` against the plugin's recorded source.
2. Filter `tag_name` by the plugin's prefix. For first-party plugins from this monorepo, the prefix is `plugins/<name>/`. For third-party plugins whose source repo does not use a prefix, the prefix is empty (match everything).
3. Drop entries where `prerelease` is `true`.
4. Strip the prefix, parse the remainder as semantic version, drop entries that fail to parse.
5. Select the maximum semantic version.
6. If the filtered list is empty, the install/update SHALL exit with `PLUGIN_FETCH_FAILED`, naming the prefix it searched for in the error's "what to do" bullets.

The plugin host SHALL NOT use the GitHub `/repos/{org}/{repo}/releases/latest` endpoint for plugin lookups, because that endpoint returns the chronologically newest non-pre-release across the entire repo regardless of tag prefix — a category mistake under prefixed plugin tags.

#### Scenario: install without --version picks max-semver of the plugin prefix

- **GIVEN** the source repo has releases `v0.6.1` (core), `plugins/triage/v0.5.0`, `v0.6.0` (core), `plugins/triage/v0.4.0`
- **WHEN** the user runs `tai plugins install triage`
- **THEN** the plugin host downloads the asset from the `plugins/triage/v0.5.0` release
- **AND** `<TAI_DATA_DIR>/state/plugins.json` records version `v0.5.0`

#### Scenario: update without --version skips pre-release

- **GIVEN** triage `v0.4.0` is installed
- **AND** the source repo's most recent triage release is `plugins/triage/v0.5.0-rc.1` (pre-release)
- **AND** no `plugins/triage/v0.5.0` stable release exists
- **WHEN** the user runs `tai plugins update triage`
- **THEN** the command exits successfully reporting "already at latest stable version"
- **AND** the installed version remains `v0.4.0`

#### Scenario: No matching plugin releases surfaces clear error

- **GIVEN** the source repo has core releases but no `plugins/triage/*` tags
- **WHEN** the user runs `tai plugins install triage`
- **THEN** the command exits with `PLUGIN_FETCH_FAILED`
- **AND** the error's "what to do" bullets name the prefix `plugins/triage/` and suggest checking the source repo's Releases page

#### Scenario: Explicit --version bypasses the lookup algorithm

- **WHEN** the user runs `tai plugins install triage --version v0.4.0`
- **THEN** the plugin host fetches the release at tag `plugins/triage/v0.4.0` directly without listing or filtering
- **AND** the install succeeds even if `v0.4.0` is older than the latest available
