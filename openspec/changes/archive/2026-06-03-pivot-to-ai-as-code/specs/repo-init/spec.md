## ADDED Requirements

### Requirement: `tai repo init <path>` scaffolds a source repo template

The system SHALL accept `tai repo init <path>` to create a new source-repo template at the given path. If `<path>` does not exist, it is created. If `<path>` exists and is non-empty, the command MUST exit with `REPO_INIT_TARGET_NOT_EMPTY` without modifying any files.

The scaffold MUST include all of the following:

- `<path>/README.md` — top-level README explaining the repo's purpose
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

#### Scenario: Scaffold rejects a non-empty target

- **WHEN** `/tmp/used` contains at least one file or subdirectory
- **AND** the user runs `tai repo init /tmp/used`
- **THEN** the command exits with `REPO_INIT_TARGET_NOT_EMPTY`
- **AND** no files in `/tmp/used` are created or modified

### Requirement: Automatic git initialization

After writing the scaffold, the system SHALL run `git init` in `<path>` and create an initial commit containing all scaffolded files with the commit message `Initial TAI source-repo scaffold`. There SHALL NOT be a flag to skip the git initialization.

If `git` is not available on `PATH`, the command MUST exit with `REPO_INIT_GIT_UNAVAILABLE` after writing the files; the files remain on disk and the user can run `git init` themselves later.

#### Scenario: Successful git init and commit

- **WHEN** the scaffold completes and `git` is available
- **THEN** `<path>/.git/` exists
- **AND** `git -C <path> log -1 --format=%s` outputs `Initial TAI source-repo scaffold`

#### Scenario: Missing git tool

- **WHEN** the scaffold completes and `git` is not on `PATH`
- **THEN** the command exits with `REPO_INIT_GIT_UNAVAILABLE`
- **AND** the scaffolded files remain on disk

### Requirement: Next-steps print block

On successful completion, the system SHALL print to stdout a multi-line next-steps block naming the exact commands the operator runs next: pushing the repo to a remote, and pointing `repo-url` at it on each consumer machine.

#### Scenario: Next-steps block format

- **WHEN** `tai repo init /tmp/my-repo` completes successfully
- **THEN** stdout contains the literal phrase `Next steps:`
- **AND** stdout contains a `git remote add origin ...` example line
- **AND** stdout contains a `tai config set repo-url ...` example line

### Requirement: No auto-wiring of local config

`tai repo init` MUST NOT modify the local TAI config. Specifically, it MUST NOT set `repo-url` to a `file://` URL, the local path, or any other value derived from the scaffolded directory. The local config remains untouched.

#### Scenario: Config untouched after init

- **WHEN** the user has a populated `~/.config/tai/config.yml` and runs `tai repo init /tmp/new-repo`
- **THEN** the config file is unchanged after the command completes
