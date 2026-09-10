# plugin-host Specification

## Purpose
TBD - created by archiving change pivot-to-ai-as-code. Update Purpose after archive.
## Requirements
### Requirement: Plugin physical layout

A plugin SHALL exist on disk at `<TAI_DATA_DIR>/plugins/<name>/`. The directory MUST contain:

- An executable file (`<name>` on Linux/macOS, `<name>.exe` on Windows) — the plugin's binary.
- A subdirectory `assets/`. The directory MUST exist (enforced at install/update time via the `PLUGIN_ASSET_MISSING` check) but MAY be empty when the plugin ships no skills, commands, or agents. When populated, it contains zero or more of: `assets/skills/`, `assets/commands/`, `assets/agents/`.

The directory name `<name>` is the single source of identity. It is also the plugin's top-level CLI verb, the namespace prefix for its skill and agent asset filenames, and the subdirectory name for its commands inside target `commands` directories. No additional manifest file is required or consulted; metadata is derived structurally.

A plugin MUST NOT write to target directories (`<target.root>/<target.commands>/`, `<target.root>/<target.skills>/`, `<target.root>/<target.agents>/`) from its own subcommands. Asset placement is owned by the host's `SyncAssetsToTargets` flow, which reads from the plugin's `assets/` directory at install/update time. A plugin SHALL NOT ship `install` / `uninstall` (or analogously-named) subverbs that bypass this rule. Enforcement is documentary (CLAUDE.md) + runtime (the mandatory `assets/` directory means the host has a guaranteed input to its own sync flow, removing the incentive for a plugin to manage its own placement).

#### Scenario: Plugin layout on disk

- **WHEN** the Triage plugin is installed at `<TAI_DATA_DIR>/plugins/triage/`
- **THEN** the directory contains an executable file `triage` (or `triage.exe`)
- **AND** the directory contains an `assets/` subdirectory (which MAY be empty for pure-binary plugins, though triage ships skills/commands)

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

The system SHALL accept `tai plugins install <name> [--source <spec>] [--version <ver>] [--yes|-y]`. The install operation:

1. Resolves `<name>` against the built-in registry of first-party plugins.
2. If unresolved AND `--source` is provided, uses the explicit source spec. The spec format is `<host>/<org>/<repo>[/<subpath>]@<version>` or compatible.
3. If unresolved AND no `--source` is given, exits with `PLUGIN_UNKNOWN`.
4. If the resolved source is NOT in the built-in registry, applies the third-party install confirmation rule (interactive prompt, `--yes` bypass, non-TTY fail). On rejection, exits with `PLUGIN_THIRDPARTY_UNCONFIRMED` before any download.
5. Fetches the platform-appropriate release asset (matching `tai-plugin-<name>-<os>-<arch>` with platform-specific suffix) via the host's Releases API.
6. Stages the tarball under a temp directory. Fails with `PLUGIN_ASSET_MISSING` if the tarball has no top-level `assets/` directory.
7. Validates plugin asset namespacing per the existing rule.
8. Captures the plugin's `--help-summary` by invoking the staged binary. Fails with `PLUGIN_HELP_SUMMARY_FAILED` on non-zero exit, empty stdout, or output exceeding 1 KB.
9. Writes the binary and `assets/` into `<TAI_DATA_DIR>/plugins/<name>/`.
10. Removes any existing files in each target's plugin namespace (skills `tai-<name>-*`, agents `tai-<name>-*`, commands `<commands>/tai-<name>/`).
11. Copies plugin assets into every configured target, applying the namespacing rules.
12. Updates `<TAI_DATA_DIR>/state/plugins.json` recording the installed source, version, install timestamp, and `--help-summary` description.
13. Prints the post-install onboarding hint on stderr (suppressed during `tai sync` auto-install — see the Post-install onboarding hint requirement).

No overwrite prompts are shown during the asset sync; the plugin owns its namespace.

If any configured target's relevant sub-path is falsy, the install SHALL skip that category for that target and warn on stderr.

#### Scenario: First-party plugin install

- **WHEN** the user runs `tai plugins install triage` and a configured target exists
- **THEN** no third-party prompt is shown (triage is built-in)
- **AND** the staged tarball contains an `assets/` directory
- **AND** `triage --help-summary` is invoked and its single-line output captured
- **AND** the binary `triage` is downloaded and placed under `<TAI_DATA_DIR>/plugins/triage/`
- **AND** assets matching the namespacing rules are written under each target
- **AND** `plugins.json` records the captured `description` field
- **AND** stderr contains the post-install onboarding hint

#### Scenario: Unknown plugin without --source

- **WHEN** the user runs `tai plugins install acme-custom` with no `--source`
- **THEN** the command exits with `PLUGIN_UNKNOWN`

#### Scenario: Third-party plugin with explicit source

- **WHEN** the user runs `tai plugins install acme-custom --source github.com/acme/tai-plugin-custom --version v1.2.0 --yes`
- **THEN** TAI fetches from the GitHub Releases of the named repo
- **AND** no interactive prompt is shown (because `--yes` was passed)
- **AND** the tarball is rejected with `PLUGIN_ASSET_MISSING` if it ships no `assets/` directory
- **AND** otherwise writes binary + assets under `<TAI_DATA_DIR>/plugins/acme-custom/`

### Requirement: `tai plugins update <name>` replaces the installed copy

`tai plugins update <name>` SHALL fetch the latest available version from the same source recorded at install time, run the same staging-time validations as install (`PLUGIN_ASSET_MISSING`, `PLUGIN_ASSET_NAMING`, `PLUGIN_HELP_SUMMARY_FAILED`), write it into the same path overwriting the previous version, wipe the plugin's namespace in each target, and re-copy current assets. State is updated with the new version, fresh install timestamp, and the freshly-captured `--help-summary` description. The post-install onboarding hint is printed on stderr.

If the recorded source is third-party AND no per-source trust has been established, the third-party install confirmation rule applies before any download (same as install).

#### Scenario: Update brings new version

- **WHEN** Triage 0.4.0 is installed and the user runs `tai plugins update triage` with 0.5.0 available
- **THEN** the binary at `<TAI_DATA_DIR>/plugins/triage/triage` is replaced
- **AND** stale namespaced assets in each target are removed and re-written
- **AND** the staged tarball is validated for `assets/` directory presence, asset namespacing, and a successful `--help-summary` capture
- **AND** the state file records version `0.5.0` AND the new description string
- **AND** stderr contains the post-install onboarding hint

### Requirement: `tai plugins remove <name>` keeps plugin data intact

`tai plugins remove <name>` SHALL:

1. Wipe the plugin's namespace in each configured target (skills, agents, commands subdir).
2. Delete the binary and `assets/` under `<TAI_DATA_DIR>/plugins/<name>/`.
3. Remove the entry from `<TAI_DATA_DIR>/state/plugins.json`.

The plugin's own runtime state (e.g. a SQLite file at `<TAI_DATA_DIR>/plugins/<name>/state/`) MUST be preserved. The command's stderr output MUST name the retained data path and remind the user to delete it manually if desired.

The same guarantee applies to install and update. `<TAI_DATA_DIR>/plugins/<name>/state/` MUST survive every install, update and remove of the plugin, with its contents unchanged. It is the ONLY path under the plugin's install directory the host preserves — the binary, `assets/`, and anything else the tarball carries are replaced wholesale, so a stale artefact can never outlive an update. An entry named `state` that is not a directory is out of contract and is replaced with the rest of the tarball's namespace.

#### Scenario: Update preserves runtime state

- **WHEN** the Triage plugin has a SQLite file at `<TAI_DATA_DIR>/plugins/triage/state/triage.db`
- **AND** the user runs `tai plugins update triage`
- **THEN** the binary and `assets/` are replaced by the new version
- **AND** `state/triage.db` is unchanged

#### Scenario: State survives a failed install

- **WHEN** an install fails after the plugin's state has been moved aside
- **THEN** the state is returned to `<TAI_DATA_DIR>/plugins/<name>/state/`
- **AND** if it cannot be returned, the error names the path holding the only surviving copy

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

At the start of `tai sync`, the system SHALL read `<clone>/plugins.yml` (if it exists). The flow:

1. Parse the entries. For each entry, resolve its source: explicit source spec if given, otherwise the built-in registry lookup.
2. If the resulting set contains AT LEAST ONE third-party source, apply the `plugins.yml` third-party trust cache rule (separate requirement). On rejection, the entire `tai sync` aborts with `PLUGIN_THIRDPARTY_UNCONFIRMED` BEFORE any plugin is installed or any asset is synced from the source repo.
3. For each entry not currently installed (per `plugins.json`), the system SHALL install it.
4. Entries already installed are not modified.
5. After all auto-installs complete successfully, sync proceeds to the source-repo asset-copy phase. If any auto-install fails (`PLUGIN_FETCH_FAILED`, `PLUGIN_ASSET_MISSING`, `PLUGIN_HELP_SUMMARY_FAILED`, etc.), `tai sync` aborts with the plugin error before the asset-copy phase begins.
6. The aggregate post-install onboarding hint (`→ <N> plugin(s) installed — run \`tai <name> help\` for any of: ...`) is printed on stderr after all auto-installs complete and before the asset-sync phase.

`plugins.yml` is additive: removing an entry SHALL NOT uninstall a plugin from the developer's machine. Removal is exclusively the user's gesture via `tai plugins remove <name>`.

#### Scenario: Auto-install of plugin listed in plugins.yml

- **WHEN** the source repo's `plugins.yml` lists `triage` and the user runs `tai sync` with `triage` not installed
- **THEN** the third-party trust check is skipped (triage is built-in)
- **AND** `triage` is installed before sync proceeds with skills/commands/agents copying
- **AND** stderr contains the aggregate onboarding hint listing `triage`

#### Scenario: plugins.yml removal does not uninstall

- **WHEN** the user has `triage` installed, and the source repo's `plugins.yml` no longer lists it
- **AND** the user runs `tai sync`
- **THEN** `triage` remains installed
- **AND** no warning is printed

#### Scenario: Third-party entry triggers trust check before any work

- **WHEN** `plugins.yml` lists `acme` from a non-built-in source
- **AND** the user runs `tai sync` with no cached trust for the repo-url
- **THEN** the trust check fires before any plugin is downloaded or installed
- **AND** on rejection, the asset-sync phase does NOT run

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

### Requirement: Mandatory `assets/` directory in every plugin tarball

A plugin tarball SHALL contain a top-level `assets/` directory. The directory MAY be empty (e.g. a pure-binary plugin that ships no skills, commands, or agents). At install and update time the system SHALL fail with `PLUGIN_ASSET_MISSING` if the directory is absent from the tarball. Empty `assets/` (no entries inside, or only an `.gitkeep`-style placeholder) is valid.

The check happens BEFORE asset-namespace validation and BEFORE the host copies anything to target directories. A missing `assets/` directory MUST short-circuit install with no on-disk side effects beyond the staging-area cleanup the host already performs on error.

This rule, combined with the rule that the host's `SyncAssetsToTargets` is the only sanctioned writer for target directories, is the trust boundary that prevents a plugin from placing files in target dirs by bypassing the host.

#### Scenario: Tarball missing `assets/` rejected

- **WHEN** a plugin tarball is downloaded that contains only the binary and `LICENSE`, with no `assets/` directory
- **THEN** install exits with `PLUGIN_ASSET_MISSING`
- **AND** the error's "what to do" bullets explain that the plugin must ship an `assets/` directory (empty is fine)
- **AND** no files are placed under `<TAI_DATA_DIR>/plugins/<name>/`

#### Scenario: Empty `assets/` directory accepted

- **WHEN** a plugin tarball is downloaded that contains the binary, `LICENSE`, and an empty `assets/` directory
- **THEN** install proceeds normally
- **AND** no errors are surfaced
- **AND** `SyncAssetsToTargets` runs vacuously (no asset categories to copy)

### Requirement: `<plugin> --help-summary` wire verb

The plugin wire contract SHALL include a `--help-summary` invocation. When run as `<plugin> --help-summary`, the plugin MUST write a single line of UTF-8 text to stdout describing the plugin in 80 characters or fewer, then exit zero. The host SHALL invoke this verb during install and update, capture the first line of stdout (truncated to 1 KB), trim ASCII whitespace, and persist it as the `description` field in the plugin's `<TAI_DATA_DIR>/state/plugins.json` entry.

If `<plugin> --help-summary` exits non-zero, writes no stdout, or the captured string exceeds 1 KB before truncation, install/update SHALL fail with `PLUGIN_HELP_SUMMARY_FAILED`. The plugin's binary and assets staged for this install MUST NOT be promoted into `<TAI_DATA_DIR>/plugins/<name>/` when this check fails; any prior install of the same plugin is left intact.

The `plugins.json` entry's `description` field is APPEND-ONLY (per the existing plugin-host data-contract rule) — once written, the schema position SHALL NOT be repurposed.

#### Scenario: First-party plugin install captures description

- **WHEN** the user runs `tai plugins install triage` and the triage binary's `--help-summary` writes `Walk through pending PR review comments interactively.`
- **THEN** install succeeds
- **AND** `<TAI_DATA_DIR>/state/plugins.json` entry for triage has `description: "Walk through pending PR review comments interactively."`

#### Scenario: Help summary failure aborts install

- **WHEN** a plugin's `--help-summary` exits with status 1
- **THEN** install exits with `PLUGIN_HELP_SUMMARY_FAILED`
- **AND** no files are placed under `<TAI_DATA_DIR>/plugins/<name>/`
- **AND** the error's "what to do" bullets name the wire verb the plugin must implement

#### Scenario: Description over 1 KB truncated and rejected

- **WHEN** a plugin's `--help-summary` writes a string longer than 1024 bytes
- **THEN** install exits with `PLUGIN_HELP_SUMMARY_FAILED`
- **AND** the error explains the 1 KB limit

### Requirement: `tai --help` lists installed plugins in a `PLUGINS:` section

The system SHALL render an additional `PLUGINS:` section in the `tai --help` output, placed after the existing `COMMANDS:` section emitted by urfave/cli. The section is sourced from `<TAI_DATA_DIR>/state/plugins.json`: one line per installed plugin, formatted as `<name>\t<description>`. The section header and trailing blank line SHALL be omitted entirely when no plugins are installed.

The section is rendered locally — no subprocess is exec'd at help-render time. The description comes from the stored `plugins.json[*].description` field captured at install/update time.

#### Scenario: Help with plugins installed

- **WHEN** triage is installed with description "Walk through pending PR review comments interactively." and no other plugins
- **AND** the user runs `tai --help`
- **THEN** stdout contains a section starting with `PLUGINS:`
- **AND** the section contains exactly one line: `triage\tWalk through pending PR review comments interactively.`

#### Scenario: Help with no plugins installed

- **WHEN** no plugins are recorded in `plugins.json`
- **AND** the user runs `tai --help`
- **THEN** stdout does NOT contain the literal `PLUGINS:` token
- **AND** no extra blank lines appear after the existing `COMMANDS:` block

### Requirement: `tai <plugin> help` forwards args to the plugin subprocess

When `<plugin>` resolves to an installed plugin, the system SHALL forward `help` (and any further args) verbatim to the plugin subprocess via the existing plugin invocation mechanism. The plugin owns the response: the host neither parses the plugin's help output nor synthesizes one. Standard streams pass through unchanged.

The reverse form `tai help <plugin>` is NOT supported. `tai help` is the reserved core verb that emits the global help; passing a plugin name as its argument SHALL result in the same global help (urfave/cli ignores unknown positional arguments after `help`).

#### Scenario: tai <plugin> help routes to the plugin

- **WHEN** triage is installed and the user runs `tai triage help`
- **THEN** the system execs `<TAI_DATA_DIR>/plugins/triage/triage help`
- **AND** stdout/stderr/exit pass through unchanged

#### Scenario: tai help <plugin> ignores the plugin name

- **WHEN** triage is installed and the user runs `tai help triage`
- **THEN** the system prints the global `tai` help to stdout
- **AND** does NOT exec the triage subprocess

### Requirement: Third-party plugin install confirmation

When `tai plugins install <name>` or `tai plugins update <name>` resolves to a source NOT in the built-in registry (i.e. a third-party plugin reached via `--source <host>/<org>/<repo>` or via a `plugins.yml` entry whose source is non-built-in), the system SHALL require explicit confirmation before downloading or staging anything.

Confirmation paths:

1. **Interactive TTY**: prompt on stderr `Installing third-party plugin <name> from <source>. Third-party plugins run arbitrary code on your machine. Continue? [y/N]`. Read stdin. Accept lowercase `y` or `yes` (trimmed); anything else (including empty input) aborts.
2. **`--yes` / `-y` flag**: bypasses the prompt for this single invocation.
3. **Non-TTY and no `--yes`**: aborts immediately with `PLUGIN_THIRDPARTY_UNCONFIRMED` without prompting. The error's "what to do" bullets MUST name the `--yes` flag.

A confirmed-but-then-failing install (e.g. fetch fails after the user said yes) SHALL leave the user's stored trust state unchanged. Confirmation is per-invocation only for the `tai plugins install`/`update` path; persistence applies only on the `tai sync` path (see the third-party trust cache requirement).

First-party plugin installs (built-in registry hits) MUST NOT trigger any prompt. The user has the right to install a first-party plugin silently.

#### Scenario: Interactive yes proceeds

- **WHEN** the user runs `tai plugins install acme --source github.com/acme/tai-plugin-acme` in a TTY
- **AND** answers `y` to the prompt
- **THEN** install proceeds normally

#### Scenario: Interactive no aborts cleanly

- **WHEN** the user runs `tai plugins install acme --source github.com/acme/tai-plugin-acme` in a TTY
- **AND** answers `N` (or empty input) to the prompt
- **THEN** the command exits with `PLUGIN_THIRDPARTY_UNCONFIRMED`
- **AND** no files are downloaded or written under `<TAI_DATA_DIR>/plugins/`

#### Scenario: --yes bypasses prompt

- **WHEN** the user runs `tai plugins install acme --source github.com/acme/tai-plugin-acme --yes`
- **THEN** no prompt is shown
- **AND** install proceeds

#### Scenario: Non-TTY without --yes fails fast

- **WHEN** `tai plugins install acme --source github.com/acme/tai-plugin-acme` runs with stdin redirected from /dev/null
- **THEN** the command exits with `PLUGIN_THIRDPARTY_UNCONFIRMED`
- **AND** stderr contains no interactive prompt
- **AND** the error names `--yes` in its "what to do" bullets

#### Scenario: First-party install never prompts

- **WHEN** the user runs `tai plugins install triage` (triage is in the built-in registry)
- **THEN** no third-party prompt is shown
- **AND** install proceeds directly

### Requirement: `plugins.yml` third-party trust cache for `tai sync`

When `tai sync` reads `<clone>/plugins.yml` (per the existing additive-auto-install rule) and the file contains AT LEAST ONE entry whose resolved source is NOT in the built-in registry, the system SHALL apply the following trust-cache logic before installing any plugin:

1. Compute `sha256` of the verbatim `plugins.yml` file bytes.
2. Load `<TAI_DATA_DIR>/state/trust.json`. Lookup the entry whose `repo-url` matches the configured `repo-url`. If present, read its `plugins-yml-sha256`.
3. If the stored hash equals the current hash, proceed with auto-install silently.
4. If the stored hash is absent or differs:
   - **Interactive TTY**: print on stderr `Source repo plugins.yml lists third-party plugins: <list of host/org/repo entries one per line>. Third-party plugins run arbitrary code on your machine. Continue? [y/N]`. Accept lowercase `y` / `yes`; anything else aborts the sync with `PLUGIN_THIRDPARTY_UNCONFIRMED`.
   - **Non-TTY and no `--trust-third-party` flag**: abort with `PLUGIN_THIRDPARTY_UNCONFIRMED` without prompting.
   - **`--trust-third-party` flag passed**: bypass the prompt as if the user said yes.
5. On user `yes` (interactive or via flag), write the new `{<repo-url>: <sha256>}` entry to `state/trust.json` BEFORE installing any plugin.

If `plugins.yml` contains zero third-party entries, the trust check SHALL be skipped entirely and no hash is computed or stored.

If aborted by `PLUGIN_THIRDPARTY_UNCONFIRMED`, no plugin is installed and `tai sync` does NOT proceed with the asset-sync phase (the rule from the existing `plugins.yml` requirement that "the host cannot reason about which assets depend on which plugin, so partial-failure modes risk producing a broken target state" applies here too).

The trust cache is keyed on `repo-url` only. A user switching `repo-url` to a different repo SHALL re-prompt independently. The cache is preserved across `repo-url` changes (entries for prior URLs are not deleted automatically; manual cleanup is the user's affair).

`state/trust.json` shape: `{"trust": [{"repo-url": "<string>", "plugins-yml-sha256": "<hex>"}, ...]}`. Schema is append-only.

#### Scenario: Internal-only plugins.yml triggers no prompt

- **WHEN** `plugins.yml` lists only built-in plugins (e.g. `triage`) and the user runs `tai sync`
- **THEN** no third-party prompt appears
- **AND** `state/trust.json` is not created or modified

#### Scenario: First sync with third-party plugins prompts

- **WHEN** `plugins.yml` lists triage AND `github.com/acme/tai-plugin-custom`
- **AND** `state/trust.json` has no entry for the configured `repo-url`
- **AND** the user runs `tai sync` in a TTY
- **THEN** the system prompts on stderr listing `acme/tai-plugin-custom` as third-party
- **AND** on `y`, writes `{<repo-url>: <sha256-of-plugins.yml>}` to `state/trust.json`
- **AND** proceeds with auto-install

#### Scenario: Unchanged plugins.yml does not re-prompt

- **WHEN** `state/trust.json` already records the current `plugins.yml` hash for the configured `repo-url`
- **AND** the user runs `tai sync`
- **THEN** no prompt is shown
- **AND** auto-install proceeds silently

#### Scenario: Modified plugins.yml re-prompts and re-stores

- **WHEN** the source repo's `plugins.yml` is edited (e.g. a new internal plugin appended)
- **AND** the cached hash no longer matches
- **AND** the user runs `tai sync` in a TTY
- **THEN** the prompt fires again
- **AND** on `y`, `state/trust.json` is updated to the new hash

#### Scenario: --trust-third-party bypasses prompt on sync

- **WHEN** the user runs `tai sync --trust-third-party` with an unsigned change in `plugins.yml`
- **THEN** no prompt is shown
- **AND** the new hash is stored in `state/trust.json`

#### Scenario: Non-TTY sync with no flag aborts

- **WHEN** `tai sync` runs with stdin redirected from /dev/null and `plugins.yml` has third-party entries that haven't been confirmed
- **THEN** the command exits with `PLUGIN_THIRDPARTY_UNCONFIRMED`
- **AND** no plugins are installed
- **AND** the asset-sync phase does NOT run

### Requirement: Post-install onboarding hint

After `tai plugins install <name>` and `tai plugins update <name>` complete successfully (regardless of whether the plugin is first-party or third-party), the system SHALL print exactly one line on stderr after the existing summary output: `→ Run \`tai <name> help\` to learn how to use <name>.`

The hint is AI-tool-agnostic — it never names a specific AI tool (Claude, Cursor, Cody, etc.). The plugin's own `help` output is where AI-tool-specific orientation lives.

The hint is suppressed when the install/update exits non-zero (any error path). It is also suppressed when `tai plugins install` is invoked as part of the auto-install flow from `tai sync` (where multiple plugins may install consecutively); in that case `tai sync` prints one aggregate `→ <N> plugin(s) installed — run \`tai <name> help\` for any of: <comma-separated names>` line at the end of the sync.

#### Scenario: Direct install prints hint

- **WHEN** `tai plugins install triage` completes successfully
- **THEN** stderr contains the line `→ Run \`tai triage help\` to learn how to use triage.`

#### Scenario: Failed install suppresses hint

- **WHEN** `tai plugins install triage` exits with `PLUGIN_FETCH_FAILED`
- **THEN** the post-install hint line is NOT printed

#### Scenario: Aggregate hint on tai sync auto-install

- **WHEN** `tai sync` auto-installs triage and `acme` from `plugins.yml`
- **THEN** the per-plugin hint is NOT printed after each install
- **AND** stderr contains one aggregate line at the end: `→ 2 plugin(s) installed — run \`tai <name> help\` for any of: triage, acme.`
