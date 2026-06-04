# workflows Specification

## Purpose
TBD - created by archiving change pivot-to-ai-as-code. Update Purpose after archive.
## Requirements
### Requirement: Workflow file format

The source repo SHALL accept YAML workflow files at `workflows/**/*.yml`. Each file MUST have:

- `description`: a one-line human-readable summary.
- `steps`: an ordered list of objects. Each object has:
  - `kind`: one of `skill` or `command`. The value `agent` is rejected.
  - `name`: a string naming the skill or command. The name is the bare identifier as it would appear after `/` in the AI's session (no leading slash in the file).

Additional top-level keys are not currently defined and MUST be rejected at load time as `WORKFLOW_INVALID`.

#### Scenario: Valid workflow

- **WHEN** TAI loads `workflows/propose.yml` containing a description and two steps with kinds `skill` and `command`
- **THEN** the workflow is accepted

#### Scenario: kind=agent rejected

- **WHEN** a workflow file has a step with `kind: agent`
- **THEN** loading exits with `WORKFLOW_INVALID`
- **AND** the error message names the offending step

### Requirement: Workflow naming and addressing

Workflows SHALL be addressed by colon-namespaced lowercased name. A workflow at `workflows/<path>.yml` resolves to the name formed by joining its path segments (relative to `workflows/`) with `:` after stripping the `.yml` suffix and lowercasing every segment. For example, `workflows/release/cut-rc.yml` resolves to `release:cut-rc`.

The names `list` and `run` are reserved as workflow names because they collide with the `tai workflow` sub-verbs. They SHALL NOT be used as a workflow name. (This is distinct from the authoritative top-level reserved verb list maintained in the `plugin-host` spec.) Duplicate names (after lowercasing) across two distinct files MUST be flagged at load time with a warning to stderr; the first file encountered wins for the addressed name.

#### Scenario: Nested workflow name

- **WHEN** a workflow exists at `workflows/release/cut-rc.yml`
- **THEN** it is addressed as `release:cut-rc`

#### Scenario: Reserved name rejected

- **WHEN** a workflow file is named `workflows/list.yml`
- **THEN** loading exits with `WORKFLOW_INVALID`
- **AND** the error names "list" as a reserved word

#### Scenario: Case-insensitive duplicate warning

- **WHEN** both `workflows/Build.yml` and `workflows/build.yml` exist
- **THEN** stderr contains a warning naming the collision
- **AND** the alphabetically earlier file wins

### Requirement: `tai workflow list`

The system SHALL print all available workflows on stdout in response to `tai workflow list`. Output format is one workflow per line: `<colon-name>  <description>`. If a workflow has no `description`, the line reads `<colon-name>  (missing description)`.

If no workflows are present in the source repo, the command prints `(no workflows)` and exits 0.

#### Scenario: List with multiple workflows

- **WHEN** `tai workflow list` runs and three workflows exist
- **THEN** stdout contains three lines, one per workflow, alphabetically ordered by name

#### Scenario: List with no workflows

- **WHEN** `tai workflow list` runs and the source repo has an empty `workflows/` directory
- **THEN** stdout contains the literal `(no workflows)`

### Requirement: `tai workflow run <name>`

The system SHALL emit a markdown plan to stdout in response to `tai workflow run <name>`. The plan MUST contain:

1. An H1 line naming the workflow.
2. The workflow's `description` as the first non-heading paragraph.
3. A "Required tools" section enumerating every step as a bulleted line of the form `<kind>:  /<name>` (kind left-justified to a stable width).
4. A "Steps" section listing every step in declaration order, numbered.
5. A "Failure mode" section instructing the AI that if any required tool is unavailable in its session, it MUST report which are missing and abort, without substituting alternatives.

If the named workflow does not exist, the command exits with `WORKFLOW_NOT_FOUND`.

#### Scenario: Run emits the contract

- **WHEN** the user runs `tai workflow run propose` and the workflow exists with two steps
- **THEN** stdout contains a "Required tools" section with both steps as bullets
- **AND** stdout contains a "Steps" section with both steps numbered
- **AND** stdout contains a "Failure mode" section instructing abort-on-missing

#### Scenario: Run on missing workflow

- **WHEN** the user runs `tai workflow run nope` and no `nope` workflow exists
- **THEN** the command exits with `WORKFLOW_NOT_FOUND`

### Requirement: Workflows are never copied to targets

The system SHALL NOT copy workflow files into any configured target during `tai sync` or any other operation. Workflow files are read on demand from the local clone only.

#### Scenario: Sync does not write workflows to target

- **WHEN** the source repo has a `workflows/propose.yml` and the user runs `tai sync`
- **THEN** no file is written under any configured target's `commands/`, `skills/`, `agents/`, or any other directory
