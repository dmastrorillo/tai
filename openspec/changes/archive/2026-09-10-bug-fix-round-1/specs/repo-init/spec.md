## MODIFIED Requirements

### Requirement: `tai repo init <path>` scaffolds a source repo template

The system SHALL accept `tai repo init <path>` to create a new source-repo template at the given path. If `<path>` does not exist, it is created. If `<path>` exists and is non-empty, the command MUST exit with `REPO_INIT_TARGET_NOT_EMPTY` without modifying any files.

The scaffold MUST include all of the following:

- `<path>/README.md` — top-level README. Its content MUST:
  - Open with a heading naming the repo as a tai source repo (`# <name> — a tai source repo`).
  - Include a one-paragraph intro explaining what tai is (a CLI for distributing AI assets across teams), so a reader who lands on the repo without having seen tai understands the context.
  - Contain a clickable link to the upstream tai project at `https://github.com/dmastrorillo/tai`.
  - NOT reference `docs.tai.sh` or any other hallucinated documentation domain — point the user at the GitHub repo README and the subfolder READMEs for further reading.
  - Retain the existing folder-layout table and the "Next steps" command block.
- `<path>/skills/README.md` — explains the skill folder, naming rules, worked example
- `<path>/commands/README.md` — explains the commands folder
- `<path>/agents/README.md` — explains the agents folder
- `<path>/workflows/README.md` — explains the YAML workflow format with an example
- `<path>/standards/README.md` — explains the markdown standards format and colon-naming
- `<path>/plugins.yml` — empty top-level list with a commented example
- `<path>/.gitignore` — opinionated defaults (OS cruft, editor scratch files)

#### Scenario: Successful scaffold into a fresh directory

- **WHEN** the user runs `tai repo init /tmp/my-repo` and `/tmp/my-repo` does not exist
- **THEN** the directory is created
- **AND** every required file and subdirectory exists with non-empty content (except where the spec says "empty")

#### Scenario: Scaffold into an existing empty directory

- **WHEN** `/tmp/empty` exists and contains zero files
- **AND** the user runs `tai repo init /tmp/empty`
- **THEN** the scaffold is written into the existing directory

#### Scenario: Scaffolded READMEs contain the conventions they document

- **WHEN** `tai repo init /tmp/my-repo` completes successfully
- **THEN** `/tmp/my-repo/skills/README.md` contains the substring `tai-<plugin>-` (the namespacing rule)
- **AND** `/tmp/my-repo/workflows/README.md` contains the substring `description:` (the YAML schema field)
- **AND** `/tmp/my-repo/standards/README.md` contains the substring `:` (colon-namespaced addressing)
- **AND** `/tmp/my-repo/plugins.yml` contains a comment line beginning with `#` (the commented example)

#### Scenario: Top-level README backlinks tai and explains the product

- **WHEN** `tai repo init /tmp/my-repo` completes successfully
- **THEN** `/tmp/my-repo/README.md` contains the substring `https://github.com/dmastrorillo/tai` (the upstream backlink)
- **AND** the README contains at least one sentence describing tai as a CLI for distributing AI assets across teams (the orientation paragraph)
- **AND** the README does NOT contain the substring `docs.tai.sh` (no hallucinated documentation domain)

#### Scenario: Scaffold rejects a non-empty target

- **WHEN** `/tmp/used` contains at least one file or subdirectory
- **AND** the user runs `tai repo init /tmp/used`
- **THEN** the command exits with `REPO_INIT_TARGET_NOT_EMPTY`
- **AND** no files in `/tmp/used` are created or modified
