// Package datadir resolves and (lazily) creates tai's global per-user
// data directory.
//
// Precedence (per add-tai-foundation/specs/cli-framework/spec.md):
//
//  1. $TAI_DATA_DIR if set and non-empty (used verbatim — no suffix).
//  2. $XDG_DATA_HOME/tai/ if $XDG_DATA_HOME is set and non-empty.
//  3. $HOME/.local/share/tai/ on Linux and macOS.
//  4. %LOCALAPPDATA%\tai\ on Windows.
//
// Resolve MUST NOT touch the filesystem. EnsureWritable creates the
// directory tree lazily and reports DATA_DIR_UNWRITABLE on failure.
package datadir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/danielmastrorillo/tai/internal/errcode"
)

// Resolve returns the path tai should use for its data directory. It
// reads environment variables and (on POSIX) the user's home directory,
// but creates no files or directories.
//
// Returns an error only when the home directory cannot be located AND
// no explicit override is set — a rare configuration on real machines.
func Resolve() (string, error) {
	if v := os.Getenv("TAI_DATA_DIR"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "tai"), nil
	}

	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "tai"), nil
		}
		// Fall back to USERPROFILE if LOCALAPPDATA is missing.
		if v := os.Getenv("USERPROFILE"); v != "" {
			return filepath.Join(v, "AppData", "Local", "tai"), nil
		}
		return "", errcode.New(errcode.DataDirUnwritable,
			"cannot resolve data directory: neither $TAI_DATA_DIR, $XDG_DATA_HOME, %LOCALAPPDATA% nor %USERPROFILE% is set").
			WithHelp("set $TAI_DATA_DIR to an absolute path")
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errcode.Wrap(errcode.DataDirUnwritable, err,
			"cannot resolve data directory: home directory is unknown").
			WithHelp("set $TAI_DATA_DIR or $HOME to an absolute path")
	}
	return filepath.Join(home, ".local", "share", "tai"), nil
}

// EnsureWritable resolves the data directory and creates it (and any
// missing parents) if absent, returning the resolved path on success.
//
// Returns a *errcode.Error{Code: DataDirUnwritable} if the directory
// cannot be created or is not writable. The remediation block points at
// $TAI_DATA_DIR as the most direct fix.
func EnsureWritable() (string, error) {
	dir, err := Resolve()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", errcode.Wrap(errcode.DataDirUnwritable, err,
			fmt.Sprintf("cannot create data directory %q", dir)).
			WithHelp(
				"check directory permissions",
				"set $TAI_DATA_DIR to a writable absolute path",
			)
	}

	// Probe writability by creating + deleting a sentinel. MkdirAll
	// reports success on an existing directory regardless of whether
	// it's writable; only an actual write proves the path is usable.
	probe, err := os.CreateTemp(dir, ".tai-writable-probe-*")
	if err != nil {
		return "", errcode.Wrap(errcode.DataDirUnwritable, err,
			fmt.Sprintf("data directory %q is not writable", dir)).
			WithHelp(
				"check directory permissions",
				"set $TAI_DATA_DIR to a writable absolute path",
			)
	}
	probeName := probe.Name()
	_ = probe.Close()
	if rmErr := os.Remove(probeName); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
		// Probe wrote but couldn't be removed — surface as an error so
		// users notice the partial-write directory before it accumulates
		// junk.
		return "", errcode.Wrap(errcode.DataDirUnwritable, rmErr,
			fmt.Sprintf("data directory %q is not fully writable: probe file remained", dir)).
			WithHelp("check directory permissions")
	}

	return dir, nil
}
