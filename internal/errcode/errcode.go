// Package errcode is the stable, append-only taxonomy of tai error codes.
//
// Every user-facing failure in tai surfaces with a code from this package
// in the error-footer line of stderr. Codes are uppercase snake-case
// identifiers. Once shipped, a code MUST NOT be renamed or repurposed;
// codes MAY be marked deprecated but their exit code and meaning MUST
// remain stable (see add-tai-foundation/specs/cli-framework/spec.md).
//
// Each code has exactly one exit code, returned by Code.ExitCode().
// Subsequent OpenSpec changes extend the taxonomy by adding new Code
// constants here AND updating the foundation spec's taxonomy table.
package errcode

import "github.com/danielmastrorillo/tai/internal/exitcode"

// Code is a stable identifier for a class of errors.
type Code string

// Foundation codes. Subsequent proposals (add-storage-schema,
// add-install-command, add-import-command, add-triage-state) append to
// this list; never reorder, never remove.
const (
	// RepoNotFound: working directory is not inside a git repo with an
	// `origin` remote, and no `--repo` flag was provided.
	RepoNotFound Code = "REPO_NOT_FOUND"

	// RepoFlagInvalid: the value passed to `--repo` does not match the
	// `<owner>/<name>` shape.
	RepoFlagInvalid Code = "REPO_FLAG_INVALID"

	// DataDirUnwritable: the resolved data directory cannot be created
	// or is not writable.
	DataDirUnwritable Code = "DATA_DIR_UNWRITABLE"

	// UnknownSubcommand: the user invoked a subcommand (or used a flag)
	// the CLI does not recognise.
	UnknownSubcommand Code = "UNKNOWN_SUBCOMMAND"

	// InternalError: unexpected internal failure (panic recovery, I/O
	// failure not anticipated by a more specific code).
	InternalError Code = "INTERNAL_ERROR"
)

// Storage-layer codes (introduced by add-storage-schema). Append-only;
// see openspec/specs/storage/spec.md for their normative meanings.
const (
	// DBOpenFailed: the database file exists or can be created, but a
	// connection-level operation (open, pragma) failed.
	DBOpenFailed Code = "DB_OPEN_FAILED"

	// DBMigrationFailed: one or more migrations failed to apply; the
	// database is in its pre-migration state.
	DBMigrationFailed Code = "DB_MIGRATION_FAILED"

	// DBConstraintViolation: an insert or update violated a NOT NULL,
	// CHECK, UNIQUE, or foreign-key constraint.
	DBConstraintViolation Code = "DB_CONSTRAINT_VIOLATION"
)

// Install-layer codes (introduced by add-install-command). Append-only;
// see openspec/changes/add-install-command/specs/install/spec.md for
// their normative meanings.
const (
	// InstallTargetUnwritable: the install target directory cannot be
	// created or is not writable.
	InstallTargetUnwritable Code = "INSTALL_TARGET_UNWRITABLE"

	// InstallInvalidTarget: the value passed to --commands-dir is
	// malformed (empty string, traversal outside a writable area).
	InstallInvalidTarget Code = "INSTALL_INVALID_TARGET"

	// InstallLedgerCorrupt: an embedded ledger file failed to parse at
	// runtime.
	InstallLedgerCorrupt Code = "INSTALL_LEDGER_CORRUPT"
)

// Import-layer codes (introduced by add-import-command). Append-only;
// see openspec/changes/add-import-command/specs/import/spec.md for
// their normative meanings.
const (
	// ImportInvalidJSON: the stdin payload is not valid JSON.
	ImportInvalidJSON Code = "IMPORT_INVALID_JSON"

	// ImportSchemaInvalid: the stdin JSON parses but fails one or more
	// schema rules.
	ImportSchemaInvalid Code = "IMPORT_SCHEMA_INVALID"

	// ImportAmbiguousRefs: a comment's external_refs resolve to more
	// than one existing comment row.
	ImportAmbiguousRefs Code = "IMPORT_AMBIGUOUS_REFS"
)

// ExitCode returns the OS exit code mapped to c. Codes outside the known
// taxonomy default to exitcode.Internal — this catches a programmer using
// a Code that was forgotten in this switch.
func (c Code) ExitCode() int {
	switch c {
	case RepoNotFound:
		return exitcode.Precondition
	case RepoFlagInvalid, UnknownSubcommand:
		return exitcode.Usage
	case DataDirUnwritable:
		return exitcode.Data
	case InternalError:
		return exitcode.Internal
	case DBOpenFailed, DBMigrationFailed, DBConstraintViolation:
		return exitcode.Data
	case InstallTargetUnwritable:
		return exitcode.Data
	case InstallInvalidTarget:
		return exitcode.Usage
	case InstallLedgerCorrupt:
		return exitcode.Internal
	case ImportInvalidJSON:
		return exitcode.Usage
	case ImportSchemaInvalid, ImportAmbiguousRefs:
		return exitcode.Data
	default:
		return exitcode.Internal
	}
}

// String returns the underlying code identifier (uppercase snake case).
func (c Code) String() string { return string(c) }
