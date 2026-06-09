# plugin-host Specification

## Purpose
TBD - created by archiving change pivot-to-ai-as-code. Update Purpose after archive.
## Requirements
### Requirement: Plugin physical layout

A plugin SHALL exist on disk at `<TAI_DATA_DIR>/plugins/<name>/`. The directory MUST contain:

- An executable file (`<name>` on Linux/macOS, `<name>.exe` on Windows) — the plugin's binary.
- A subdirectory `assets/` containing zero or more of: `assets/skills/`, `assets/commands/`, `assets/agents/`.

The directory name `<name>` is the single source of identity. It is also the plugin's top-level CLI verb, the namespace prefix for its skill and agent asset filenames, and the subdirectory name for its commands inside target `commands` directories. No additional manifest file is required or consulted; metadata is derived structurally.

#### Scenario: Plugin layout on disk

- **WHEN** the Triage plugin is installed at `<TAI_DATA_DIR>/plugins/triage/`
- **THEN** the directory contains an executable file `triage` (or `triage.exe`)
- **AND** the directory contains an `assets/` subdirectory

### Requirement: Plugin subprocess invocation

When the user runs `tai <name> <args...>` and `<name>` is not a reserved core verb, the system SHALL:

1. Look up `<name>` in the local plugin registry state.
2. If found, execute the plugin's binary with `<args...>` as `argv` from index 1.
3. Pass through stdin, stdout, stderr, and exit code unchanged.

If `<name>` is not found, the command MUST exit with `UNKNOWN_SUBCOMMAND` and the "what to do" bullets MUST suggest `tai plugins list` and `tai plugins install <name>`.

The authoritative list of reserved top-level verbs is maintained here: `config`, `sync`, `repo`, `install-commands`, `workflow`, `standards`, `plugins`, `help`, `version`. Other specs and the implementation registry MUST reference this list rather than duplicating it. A plugin installation MUST fail at install time with `PLUGIN_NAME_RESERVED` if the plugin name collides with any entry in this list. New top-level verbs added to TAI MUST be appended to this list in the same proposal that introduces them.

(The reserved names used inside the `tai workflow` and `tai standards` namespaces — `list`, `run`, `load` — are scoped sub-verb collisions and live in their respective specs; they are unrelated to this top-level list.)

#### Scenario: Plugin invocation

- **WHEN** the Triage plugin is installed and the user runs `tai triage list`
- **THEN** the system executes `<TAI_DATA_DIR>/plugins/triage/triage list`
- **AND** the plugin's stdout/stderr/exit are surfaced to the user as-is

#### Scenario: Unknown plugin name

- **WHEN** the user runs `tai nope` and no plugin `nope` is installed
- **THEN** the command exits with `UNKNOWN_SUBCOMMAND`

#### Scenario: Reserved-verb collision at install

- **WHEN** the user attempts to install a plugin whose name is `config`
- **THEN** the command exits with `PLUGIN_NAME_RESERVED`
- **AND** no files are written under `<TAI_DATA_DIR>/plugins/`

### Requirement: Plugin environment-variable contract

When invoking a plugin subprocess, the system SHALL set the following environment variables in addition to the inherited environment:

- `TAI_CLONE_DIR`: absolute path to the source-repo clone, or empty if no source repo is configured.
- `TAI_TARGETS`: JSON array serializing the configured targets in the form `[{"root":"...","skills":"...","commands":"...","agents":"..."}, ...]` with effective (defaulted) sub-paths.
- `TAI_DATA_DIR`: absolute path to TAI's data directory.

These variables are part of the plugin wire contract and SHALL NOT be renamed without a major version bump. Additional `TAI_*` variables MAY be added in the future; plugin authors MUST NOT rely on the absence of unknown `TAI_*` variables.

#### Scenario: Env vars passed to plugin

- **WHEN** TAI invokes a plugin with one configured target `~/.claude` and a configured source repo
- **THEN** the child process's environment contains `TAI_CLONE_DIR` set to the absolute clone path
- **AND** `TAI_TARGETS` is a JSON array with one object whose `root` field is `~/.claude`
- **AND** `TAI_DATA_DIR` is set to the absolute data directory path

### Requirement: Plugin asset namespacing

Plugin assets installed into target directories SHALL follow these naming rules, enforced by TAI:

- **Skills**: each file/folder in the plugin's `assets/skills/` MUST be named starting with `tai-<plugin>-`. Install fails with `PLUGIN_ASSET_NAMING` if any does not match.
- **Agents**: each file in the plugin's `assets/agents/` MUST be named starting with `tai-<plugin>-`. Same validation rule.
- **Commands**: filenames in the plugin's `assets/commands/` are unconstrained. TAI writes them into `<target.root>/<target.commands>/tai-<plugin>/` regardless of authored name.

#### Scenario: Skill prefix enforced at install

- **WHEN** the user attempts to install a plugin `mytool` whose `assets/skills/foo.md` does not start with `tai-mytool-`
- **THEN** the install exits with `PLUGIN_ASSET_NAMING`
- **AND** the error names the offending file

#### Scenario: Commands routed into namespaced subdir

- **WHEN** the user installs the Triage plugin whose `assets/commands/` contains `import.md`
- **THEN** the target receives the file at `<target.root>/<target.commands>/tai-triage/import.md`

### Requirement: `tai plugins install <name>` performs install plus asset sync

The system SHALL accept `tai plugins install <name> [--source <spec>] [--version <ver>]`. The install operation:

1. Resolves `<name>` against the built-in registry of first-party plugins.
2. If unresolved AND `--source` is provided, uses the explicit source spec. The spec format is `<host>/<org>/<repo>[/<subpath>]@<version>` or compatible.
3. If unresolved AND no `--source` is given, exits with `PLUGIN_UNKNOWN`.
4. Fetches the platform-appropriate release asset (matching `tai-plugin-<name>-<os>-<arch>` with platform-specific suffix) via the host's Releases API.
5. Writes the binary and `assets/` into `<TAI_DATA_DIR>/plugins/<name>/`.
6. Validates plugin asset namespacing per the previous requirement.
7. Removes any existing files in each target's plugin namespace (skills `tai-<name>-*`, agents `tai-<name>-*`, commands `<commands>/tai-<name>/`).
8. Copies plugin assets into every configured target, applying the namespacing rules.
9. Updates `<TAI_DATA_DIR>/state/plugins.json` recording the installed source, version, and install timestamp.

No overwrite prompts are shown during the asset sync; the plugin owns its namespace.

If any configured target's relevant sub-path is falsy, the install SHALL skip that category for that target and warn on stderr.

#### Scenario: First-party plugin install

- **WHEN** the user runs `tai plugins install triage` and a configured target exists
- **THEN** the binary `triage` is downloaded and placed under `<TAI_DATA_DIR>/plugins/triage/`
- **AND** assets matching the namespacing rules are written under each target

#### Scenario: Unknown plugin without --source

- **WHEN** the user runs `tai plugins install acme-custom` with no `--source`
- **THEN** the command exits with `PLUGIN_UNKNOWN`

#### Scenario: Third-party plugin with explicit source

- **WHEN** the user runs `tai plugins install acme-custom --source github.com/acme/tai-plugin-custom --version v1.2.0`
- **THEN** TAI fetches from the GitHub Releases of the named repo
- **AND** writes binary + assets under `<TAI_DATA_DIR>/plugins/acme-custom/`

### Requirement: `tai plugins update <name>` replaces the installed copy

`tai plugins update <name>` SHALL fetch the latest available version from the same source recorded at install time, write it into the same path overwriting the previous version, wipe the plugin's namespace in each target, and re-copy current assets. State is updated with the new version and a fresh install timestamp.

#### Scenario: Update brings new version

- **WHEN** Triage 0.4.0 is installed and the user runs `tai plugins update triage` with 0.5.0 available
- **THEN** the binary at `<TAI_DATA_DIR>/plugins/triage/triage` is replaced
- **AND** stale namespaced assets in each target are removed and re-written
- **AND** the state file records version `0.5.0`

### Requirement: `tai plugins remove <name>` keeps plugin data intact

`tai plugins remove <name>` SHALL:

1. Wipe the plugin's namespace in each configured target (skills, agents, commands subdir).
2. Delete the binary and `assets/` under `<TAI_DATA_DIR>/plugins/<name>/`.
3. Remove the entry from `<TAI_DATA_DIR>/state/plugins.json`.

The plugin's own runtime state (e.g. a SQLite file at `<TAI_DATA_DIR>/plugins/<name>/state/`) MUST be preserved. The command's stderr output MUST name the retained data path and remind the user to delete it manually if desired.

#### Scenario: Remove preserves runtime state

- **WHEN** the Triage plugin has a SQLite file at `<TAI_DATA_DIR>/plugins/triage/state/triage.db`
- **AND** the user runs `tai plugins remove triage`
- **THEN** the binary, assets/ folder, and namespaced target files are deleted
- **AND** the SQLite file at the state path remains
- **AND** stderr names the retained path

### Requirement: `tai plugins list`

`tai plugins list` SHALL print a table on stdout listing every installed plugin with columns `name`, `version`, `installed-at`. If no plugins are installed, prints `(no plugins installed)`.

#### Scenario: Listing installed plugins

- **WHEN** Triage 0.5.0 is installed and no other plugins are
- **THEN** `tai plugins list` outputs a header line and one data row containing `triage` and `0.5.0`

### Requirement: `plugins.yml` additive auto-install on sync

At the start of `tai sync`, the system SHALL read `<clone>/plugins.yml` (if it exists). For each entry not currently installed, the system SHALL install it before proceeding with asset sync. Entries already installed are not modified.

`plugins.yml` is additive: removing an entry SHALL NOT uninstall a plugin from the developer's machine. Removal is exclusively the user's gesture via `tai plugins remove <name>`.

#### Scenario: Auto-install of plugin listed in plugins.yml

- **WHEN** the source repo's `plugins.yml` lists `triage` and the user runs `tai sync` with `triage` not installed
- **THEN** `triage` is installed before sync proceeds with skills/commands/agents copying

#### Scenario: plugins.yml removal does not uninstall

- **WHEN** the user has `triage` installed, and the source repo's `plugins.yml` no longer lists it
- **AND** the user runs `tai sync`
- **THEN** `triage` remains installed
- **AND** no warning is printed

### Requirement: Opportunistic `GITHUB_TOKEN`

When fetching plugin release assets from `github.com`, the system SHALL include the value of `$GITHUB_TOKEN` (when set and non-empty) as a Bearer token. When unset, requests are anonymous. On 401 or 403, the error message MUST name `GITHUB_TOKEN` as the resolution path.

#### Scenario: Anonymous fetch succeeds for public repo

- **WHEN** `GITHUB_TOKEN` is unset and the plugin source is a public repo
- **THEN** the fetch succeeds with no Authorization header

#### Scenario: 401 surfaces actionable error

- **WHEN** a plugin source is private and `GITHUB_TOKEN` is unset
- **THEN** the install exits with `PLUGIN_FETCH_UNAUTHORIZED`
- **AND** the "what to do" bullets mention setting `GITHUB_TOKEN`

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
