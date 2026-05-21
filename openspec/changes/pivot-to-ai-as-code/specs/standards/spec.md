## ADDED Requirements

### Requirement: Standards file format

The source repo SHALL accept markdown files at `standards/**/*.md` as team-wide standards documents. Each file MAY include YAML frontmatter at the top with a `description:` field. Content is opaque to TAI — TAI does not interpret it.

#### Scenario: Standard with frontmatter description

- **WHEN** TAI loads `standards/SDLC.md` whose frontmatter contains `description: Software development lifecycle`
- **THEN** the parsed description is `Software development lifecycle`

#### Scenario: Standard without frontmatter

- **WHEN** TAI loads `standards/SDLC.md` with no frontmatter
- **THEN** the parsed description is the literal string `(missing description in frontmatter)`

### Requirement: Standards naming and addressing

Standards are addressed by colon-namespaced lowercased name. A standard at `standards/<path>.md` resolves to the name formed by joining its path segments (relative to `standards/`) with `:` after stripping the `.md` suffix and lowercasing every segment. For example, `standards/devOps/security/best-practices.md` resolves to `devops:security:best-practices`.

The names `list` and `load` are reserved as standard names because they collide with the `tai standards` sub-verbs. (This is distinct from the authoritative top-level reserved verb list maintained in the `plugin-host` spec.) Two files whose lowercased names collide MUST emit a stderr warning at load time; the alphabetically earlier file wins for the addressed name.

#### Scenario: Nested standard name

- **WHEN** a standard exists at `standards/devOps/security/best-practices.md`
- **THEN** it is addressed as `devops:security:best-practices`

#### Scenario: Reserved name rejected

- **WHEN** a standard file at `standards/list.md` is loaded
- **THEN** the load exits with `STANDARD_INVALID` naming "list" as a reserved word

#### Scenario: Case-insensitive collision warning

- **WHEN** both `standards/Foo.md` and `standards/foo.md` exist
- **THEN** stderr contains a warning naming the collision
- **AND** the alphabetically earlier file resolves the name

### Requirement: `tai standards list`

The system SHALL print all available standards on stdout in response to `tai standards list`. Output format is one standard per line: `<colon-name>  <description>` with the description column aligned by spaces. Standards are listed in alphabetical order by colon-name.

If no standards are present, the command prints `(no standards)` and exits 0.

#### Scenario: List with multiple standards at varying depths

- **WHEN** the source repo has `standards/SDLC.md`, `standards/devOps/security/best-practices.md`
- **THEN** `tai standards list` lists `devops:security:best-practices` and `sdlc` in alphabetical order

### Requirement: `tai standards load <name>`

The system SHALL print the body of the named standard (with frontmatter stripped) to stdout in response to `tai standards load <name>`. If the named standard does not exist, the command exits with `STANDARD_NOT_FOUND`. The body is emitted byte-for-byte after frontmatter removal — no transformation.

#### Scenario: Load returns body

- **WHEN** a standard `standards/SDLC.md` exists with `description:` frontmatter and a body section
- **AND** the user runs `tai standards load sdlc`
- **THEN** stdout contains the body content
- **AND** the frontmatter does not appear in stdout

#### Scenario: Load on missing name

- **WHEN** the user runs `tai standards load nonexistent`
- **THEN** the command exits with `STANDARD_NOT_FOUND`

### Requirement: Standards are never copied to targets

The system SHALL NOT copy standards files into any configured target during `tai sync` or any other operation. They are read on demand from the local clone only.

#### Scenario: Sync does not write standards to target

- **WHEN** the source repo has standards and the user runs `tai sync`
- **THEN** no standards file appears in any target directory
