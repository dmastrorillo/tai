## ADDED Requirements

### Requirement: Verb-to-slash-command pairing

The system SHALL maintain a one-to-one pairing between user-facing CLI verbs and bundled Claude slash commands. For every verb `tai <verb>` intended to be driven by Claude, the system MUST ship a slash command `/tai:<verb>` whose markdown file is installed at `~/.claude/commands/tai/<verb>.md`.

A verb MAY exist without a paired slash command (for example `tai install` is invoked by humans only). A slash command MUST NOT exist without a corresponding CLI verb.

#### Scenario: Every bundled command has a verb

- **WHEN** the binary ships with a bundled slash command `/tai:import`
- **THEN** the CLI provides a `tai import` subcommand

#### Scenario: A verb may have no slash command

- **WHEN** the CLI provides a `tai install` subcommand
- **THEN** the binary is not required to bundle a `/tai:install` slash command

### Requirement: Slash-command file location

The system SHALL install bundled slash-command markdowns at `~/.claude/commands/tai/<verb>.md`, where `<verb>` is the CLI verb in lowercase kebab-case. The directory `~/.claude/commands/tai/` MUST be created if it does not exist.

The exact install command behaviour (idempotency, overriding the target directory, prompting on user-modified files) is specified in the `install` capability and is out of scope for this requirement.

#### Scenario: Bundled import command target path

- **WHEN** the bundled slash command for `tai import` is installed under default paths
- **THEN** the file is written to `~/.claude/commands/tai/import.md`

### Requirement: Slash-command frontmatter schema

Every bundled slash-command markdown SHALL begin with a YAML frontmatter block bounded by `---` lines, containing the following fields:

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `name` | string | yes | Human-readable command name, e.g. `"TAI: Import"`. |
| `description` | string | yes | One-line description shown in command pickers. |
| `category` | string | yes | Top-level grouping; tai commands use `"Workflow"`. |
| `tags` | string array | yes | Searchable tags. Every tai command includes `tai` as the first tag. |
| `version` | integer | yes | Monotonically incrementing version, scoped per command. Starts at `1`. |
| `content_hash` | string | yes | `sha256:<hex>` of the body bytes (everything after the closing frontmatter `---`) shipped in this build. |

Fields not listed here MUST NOT appear in the frontmatter of a bundled command. Future additions require a foundation-level proposal that extends this schema.

#### Scenario: Valid frontmatter

- **WHEN** a bundled command ships with frontmatter containing all six required fields and no extras
- **THEN** the file is well-formed under the schema

#### Scenario: Missing required field

- **WHEN** the build pipeline produces a command without a `content_hash` field
- **THEN** the build MUST fail

### Requirement: Per-command versioning is independent of CLI semver

The system SHALL version each bundled slash command with its own monotonic integer `version`, independent of the CLI's overall semver release version.

A CLI release that does not change a given command's body MUST NOT increment that command's `version`. A CLI release that changes a command's body MUST increment its `version` by exactly one.

#### Scenario: CLI release without command changes

- **WHEN** CLI version `0.4.0` is released and the `/tai:import` body is byte-identical to the body shipped in `0.3.0`
- **THEN** the `/tai:import` frontmatter `version` field is the same integer in both releases

#### Scenario: Command body changes increment version

- **WHEN** CLI version `0.5.0` is released with a modified `/tai:import` body
- **THEN** the `/tai:import` frontmatter `version` field is exactly one greater than the value shipped in `0.4.0`

### Requirement: Content hash is computed over body bytes only

The system SHALL compute each bundled command's `content_hash` as the sha256 hex digest of the body bytes — defined as the file contents starting immediately after the line containing the closing frontmatter `---` and continuing to the end of the file.

The `---` delimiter line itself is NOT included in the hash input. A trailing newline at the end of the file IS included if present.

The hash format in the frontmatter is `sha256:<64 lowercase hex chars>`.

#### Scenario: Hash excludes frontmatter

- **WHEN** the frontmatter `description` field is edited but the body is unchanged
- **THEN** the `content_hash` value remains the same

#### Scenario: Hash includes trailing newline

- **WHEN** two otherwise-identical command bodies differ only by the presence of a trailing newline
- **THEN** their `content_hash` values differ

### Requirement: Embedded cumulative hash ledger per command

The system SHALL embed, for every bundled slash command, the cumulative ledger of every `content_hash` value ever shipped for that command across all prior releases. The ledger is generated at build time from the previous release's ledger plus the hashes added by the current build.

The ledger MUST be accessible to the CLI at runtime via `//go:embed` (or equivalent) so that `tai install` can perform the file-state classification specified in the `install` capability.

The exact serialisation format of the ledger (filename, JSON shape) is an implementation detail and may be defined in the `install` capability.

#### Scenario: Ledger includes the current build's hash

- **WHEN** CLI version `0.5.0` is built and the `/tai:import` body produces hash `sha256:abc…`
- **THEN** the ledger embedded for `/tai:import` includes `sha256:abc…` as one of its entries

#### Scenario: Ledger preserves prior entries

- **WHEN** CLI version `0.5.0` is built and CLI version `0.4.0` shipped `/tai:import` with hash `sha256:def…`
- **THEN** the ledger embedded for `/tai:import` in `0.5.0` includes `sha256:def…` as one of its entries

#### Scenario: Renamed command resets the ledger

- **WHEN** a command is renamed from `tai foo` to `tai bar` (deliberate breaking change)
- **THEN** the ledger embedded for `tai bar` does not include hashes that were previously associated with `tai foo`

### Requirement: Slash-command body delegates state to the CLI

Every bundled slash command's body SHALL drive a conversation in Claude and delegate persistence and state mutation to the CLI by shelling out to `tai <verb>` (and related raw verbs).

The body MUST NOT duplicate logic that the CLI implements (parsing, validation, storage). Its role is to orchestrate Claude's behaviour and invoke the CLI for everything that touches state.

#### Scenario: Import command body shells out

- **WHEN** the user invokes `/tai:import 142`
- **THEN** the slash command body instructs Claude to call `tai import -` (or related verbs) for persistence rather than writing files directly
