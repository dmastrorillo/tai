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

import "github.com/dmastrorillo/tai/pkg/exitcode"

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

// Config codes (introduced by pivot-to-ai-as-code Phase 1). Append-only;
// see openspec/changes/pivot-to-ai-as-code/specs/config/spec.md for their
// normative meanings.
const (
	// ConfigUnwritable: the resolved config file cannot be created, or
	// its parent directory is not writable.
	ConfigUnwritable Code = "CONFIG_UNWRITABLE"

	// ConfigInvalid: the config file parses as YAML but violates a
	// structural rule (e.g. a target with every sub-path set falsy).
	ConfigInvalid Code = "CONFIG_INVALID"

	// ConfigInvalidRepoURL: `repo-url` is not a remote git URL. Local
	// paths and `file://` URLs are rejected.
	ConfigInvalidRepoURL Code = "CONFIG_INVALID_REPO_URL"

	// ConfigKeyNotScriptable: `tai config set` was invoked on a nested
	// or array key. Use a dedicated subcommand or `tai config edit`.
	ConfigKeyNotScriptable Code = "CONFIG_KEY_NOT_SCRIPTABLE"

	// ConfigDuplicateTarget: `tai config target add` was invoked for a
	// `root` that already exists in the targets array.
	ConfigDuplicateTarget Code = "CONFIG_DUPLICATE_TARGET"

	// ConfigTargetNotFound: `tai config target remove` was invoked for a
	// `root` that does not exist in the targets array.
	ConfigTargetNotFound Code = "CONFIG_TARGET_NOT_FOUND"

	// ConfigEditorUnset: `tai config edit` was invoked with no $EDITOR
	// environment variable set.
	ConfigEditorUnset Code = "CONFIG_EDITOR_UNSET"

	// TaiNotConfigured: an operation requiring both `repo-url` and
	// `targets` was run with at least one missing.
	TaiNotConfigured Code = "TAI_NOT_CONFIGURED"

	// MissingArg: a subcommand was invoked with the wrong number of
	// positional arguments (typically too few). The user picked a real
	// verb; the error is about arity, not unrecognised input.
	//
	// Distinct from UnknownSubcommand (verb not in the registry) and
	// from CONFIG_KEY_NOT_SCRIPTABLE (key shape rejected). Exit code
	// is Usage (1).
	MissingArg Code = "MISSING_ARG"
)

// Repo-sync and repo-init codes (both introduced by
// pivot-to-ai-as-code Phase 2). Append-only; see
// openspec/changes/pivot-to-ai-as-code/specs/{repo-sync,repo-init}/
// spec.md for the normative meanings.
const (
	// RepoFetchFailed: `git fetch` against the configured source repo
	// failed for reasons other than network-unreachable (e.g.
	// authentication, 4xx). Network-unreachable is handled implicitly
	// via the cache-fallback warning and does NOT surface as an error.
	RepoFetchFailed Code = "REPO_FETCH_FAILED"

	// RepoInitTargetNotEmpty: `tai repo init <path>` was invoked
	// against an existing directory that already contains files. The
	// scaffold refuses to overwrite — the user must delete or move the
	// existing contents.
	RepoInitTargetNotEmpty Code = "REPO_INIT_TARGET_NOT_EMPTY"

	// RepoInitGitUnavailable: `tai repo init` finished writing the
	// scaffold but `git` is not on PATH, so the auto `git init` +
	// initial commit step cannot run. Files remain on disk; the user
	// can run `git init` themselves once git is installed.
	RepoInitGitUnavailable Code = "REPO_INIT_GIT_UNAVAILABLE"
)

// Workflow and standards codes (introduced by pivot-to-ai-as-code
// Phase 6 — content surfaces). Append-only; see
// openspec/changes/pivot-to-ai-as-code/specs/{workflows,standards}/
// spec.md for the normative meanings.
const (
	// WorkflowInvalid: a workflow YAML file under `<clone>/workflows/`
	// violates the schema (missing required fields, unknown top-level
	// key, `kind: agent`, reserved name `list`/`run`).
	WorkflowInvalid Code = "WORKFLOW_INVALID"

	// WorkflowNotFound: `tai workflow run <name>` was invoked for a
	// name that does not resolve to any loaded workflow.
	WorkflowNotFound Code = "WORKFLOW_NOT_FOUND"

	// StandardInvalid: a standard file under `<clone>/standards/`
	// uses a reserved name (`list`, `load`).
	StandardInvalid Code = "STANDARD_INVALID"

	// StandardNotFound: `tai standards load <name>` was invoked for a
	// name that does not resolve to any loaded standard.
	StandardNotFound Code = "STANDARD_NOT_FOUND"
)

// Plugin-host codes (introduced by pivot-to-ai-as-code Phase 4 —
// plugin host). Append-only; see
// openspec/specs/plugin-host/spec.md for the normative meanings.
const (
	// PluginUnknown: `tai plugins install <name>` was invoked for a
	// name that has no entry in the built-in first-party registry
	// AND no `--source` flag was supplied.
	PluginUnknown Code = "PLUGIN_UNKNOWN"

	// PluginNameReserved: the user attempted to install a plugin
	// whose name collides with a reserved top-level verb
	// (config, sync, repo, install-commands, workflow, standards,
	// plugins, help, version).
	PluginNameReserved Code = "PLUGIN_NAME_RESERVED"

	// PluginAssetNaming: a plugin's downloaded `assets/skills/` or
	// `assets/agents/` contains an entry whose name does not start
	// with the mandatory `tai-<plugin>-` prefix.
	PluginAssetNaming Code = "PLUGIN_ASSET_NAMING"

	// PluginFetchUnauthorized: the release-asset fetch returned 401
	// or 403. Distinct from PluginFetchFailed because the
	// remediation is specific: set $GITHUB_TOKEN (or the host's
	// equivalent credential env var).
	PluginFetchUnauthorized Code = "PLUGIN_FETCH_UNAUTHORIZED"

	// PluginFetchFailed: the release-asset fetch failed for a reason
	// other than auth (5xx, network unreachable, malformed asset).
	PluginFetchFailed Code = "PLUGIN_FETCH_FAILED"
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
// see openspec/specs/import/spec.md for their normative meanings.
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

// Triage-layer codes (introduced by add-triage-state). Append-only;
// see openspec/changes/add-triage-state/specs/triage/spec.md for their
// normative meanings.
const (
	// TriageNoScope: the current branch matches no PR and no branch row,
	// and no --pr/--branch was provided.
	TriageNoScope Code = "TRIAGE_NO_SCOPE"

	// TriageAmbiguousScope: the current branch matches both a
	// prs.head_branch and a branches.name row.
	TriageAmbiguousScope Code = "TRIAGE_AMBIGUOUS_SCOPE"

	// TriageNotFound: the referenced PR, branch, comment, or batch does
	// not exist in the resolved scope.
	TriageNotFound Code = "TRIAGE_NOT_FOUND"

	// TriageInvalidFlags: conflicting or missing flags on a triage verb
	// (e.g. --pr + --branch, missing --reason, --id + --batch).
	TriageInvalidFlags Code = "TRIAGE_INVALID_FLAGS"

	// TriageConfirmationRequired: tai forget was invoked non-
	// interactively without --yes or a truthy TAI_ACCEPT_DESTRUCTIVE.
	TriageConfirmationRequired Code = "TRIAGE_CONFIRMATION_REQUIRED"
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
	case ConfigUnwritable:
		return exitcode.Data
	case ConfigInvalid, ConfigInvalidRepoURL, ConfigKeyNotScriptable,
		ConfigDuplicateTarget, ConfigTargetNotFound, ConfigEditorUnset:
		return exitcode.Usage
	case TaiNotConfigured:
		return exitcode.Precondition
	case MissingArg:
		return exitcode.Usage
	case RepoFetchFailed:
		return exitcode.Data
	case RepoInitTargetNotEmpty:
		return exitcode.Usage
	case RepoInitGitUnavailable:
		return exitcode.Data
	case WorkflowInvalid, StandardInvalid:
		return exitcode.Usage
	case WorkflowNotFound, StandardNotFound:
		return exitcode.Precondition
	case PluginUnknown, PluginNameReserved, PluginAssetNaming,
		PluginFetchUnauthorized:
		return exitcode.Usage
	case PluginFetchFailed:
		return exitcode.Data
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
	case TriageNoScope, TriageAmbiguousScope, TriageNotFound:
		return exitcode.Precondition
	case TriageInvalidFlags, TriageConfirmationRequired:
		return exitcode.Usage
	default:
		return exitcode.Internal
	}
}

// String returns the underlying code identifier (uppercase snake case).
func (c Code) String() string { return string(c) }
