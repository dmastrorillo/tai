// Package repoctx resolves the current repository's `owner/name`
// identity from the working directory's git remote — or returns a
// structured error pointing the user at `--repo` when the working
// directory is not a usable git repo.
//
// The parser is pure (string-in → string-or-error-out); the reader
// shells out to `git config --get remote.origin.url`. No third-party git
// library; std `os/exec` is sufficient.
package repoctx

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"

	"github.com/dmastrorillo/tai/internal/errcode"
)

// Identity is a normalised repo identity, always of the form
// `<owner>/<name>` (e.g. "acme/app").
type Identity string

// Owner returns the owner segment (before the slash).
func (i Identity) Owner() string {
	idx := strings.IndexByte(string(i), '/')
	if idx < 0 {
		return string(i)
	}
	return string(i)[:idx]
}

// Name returns the name segment (after the slash).
func (i Identity) Name() string {
	idx := strings.IndexByte(string(i), '/')
	if idx < 0 {
		return ""
	}
	return string(i)[idx+1:]
}

// String returns the canonical `<owner>/<name>` form.
func (i Identity) String() string { return string(i) }

// identityRe enforces the canonical `<owner>/<name>` shape used by the
// `--repo` flag validator. Owner and name both contain at least one
// character and no slash.
var identityRe = regexp.MustCompile(`^([^/\s]+)/([^/\s]+)$`)

// ParseIdentity returns an Identity from a raw `<owner>/<name>` string,
// or a *errcode.Error{Code: RepoFlagInvalid} if the value does not
// match the canonical shape.
func ParseIdentity(s string) (Identity, error) {
	if !identityRe.MatchString(s) {
		return "", errcode.Newf(errcode.RepoFlagInvalid,
			"value %q does not match the expected <owner>/<name> format", s).
			WithHelp(
				"use a value like `acme/app`",
				"`<owner>` and `<name>` must each contain at least one character and no slash",
			)
	}
	return Identity(s), nil
}

// ParseOriginURL extracts the normalised `<owner>/<name>` identity from
// a remote URL string. It accepts these forms (with or without a
// trailing `.git`):
//
//   - git@github.com:acme/app.git
//   - ssh://git@github.com/acme/app.git
//   - https://github.com/acme/app.git
//   - https://github.com/acme/app
//
// Returns an empty Identity and a non-nil error when the URL cannot be
// parsed. The returned error is intentionally NOT a *errcode.Error —
// callers (chiefly Read) decide whether the failure means
// RepoNotFound or something else.
func ParseOriginURL(raw string) (Identity, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("origin URL is empty")
	}

	// Strip scheme + host portion to leave "owner/name(.git)?".
	tail := raw
	switch {
	case strings.HasPrefix(tail, "git@"):
		// SCP-style: git@host:owner/name(.git)?
		if colon := strings.IndexByte(tail, ':'); colon >= 0 {
			tail = tail[colon+1:]
		} else {
			return "", errors.New("malformed SCP-style URL: missing ':'")
		}
	case strings.Contains(tail, "://"):
		// scheme://host/owner/name(.git)?
		afterScheme := tail[strings.Index(tail, "://")+3:]
		slash := strings.IndexByte(afterScheme, '/')
		if slash < 0 {
			return "", errors.New("malformed URL: no path component")
		}
		tail = afterScheme[slash+1:]
	default:
		return "", errors.New("unrecognised remote URL form")
	}

	tail = strings.TrimSuffix(tail, ".git")
	tail = strings.TrimSuffix(tail, "/")

	if !identityRe.MatchString(tail) {
		return "", errors.New("URL path is not <owner>/<name>")
	}
	return Identity(tail), nil
}

// Read resolves the current working directory's repo identity by
// reading `git config --get remote.origin.url` and parsing the result.
//
// Returns a *errcode.Error{Code: RepoNotFound} when:
//
//   - The directory is not inside any git repository, OR
//   - The repository has no `origin` remote, OR
//   - The `origin` remote's URL cannot be parsed into <owner>/<name>.
//
// Returns a *errcode.Error{Code: InternalError} when `git` is not on
// the user's PATH — that is an environment problem, not a missing-repo
// problem, and the user can't fix it by changing directories.
//
// The help text on the returned error points the user at the most
// direct remediation for the specific failure mode.
func Read(ctx context.Context) (Identity, error) {
	out, err := exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url").Output()
	if err != nil {
		// `git` not on PATH — different remediation from "not a repo".
		if errors.Is(err, exec.ErrNotFound) {
			return "", errcode.Wrap(errcode.InternalError, err,
				"`git` not found on PATH").
				WithHelp(
					"install git",
					"or pass --repo <owner/name> explicitly to skip auto-detection",
				)
		}

		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// `git config --get` exits 1 when the key is unset; non-1
			// exits typically mean we're not inside a git repo.
			if ee.ExitCode() == 1 {
				return "", errcode.New(errcode.RepoNotFound,
					"no `origin` remote configured for this repository").
					WithHelp(
						"add a remote: `git remote add origin <url>`",
						"or pass --repo <owner/name> explicitly",
					)
			}
		}
		return "", errcode.Wrap(errcode.RepoNotFound, err,
			"not inside a git repository").
			WithHelp(
				"cd into a git repository, or",
				"pass --repo <owner/name> explicitly",
			)
	}

	id, parseErr := ParseOriginURL(strings.TrimSpace(string(out)))
	if parseErr != nil {
		return "", errcode.Wrapf(errcode.RepoNotFound, parseErr,
			"could not parse origin URL %q as <owner>/<name>", strings.TrimSpace(string(out))).
			WithHelp(
				"check `git config --get remote.origin.url`",
				"or pass --repo <owner/name> explicitly",
			)
	}
	return id, nil
}

// Resolve returns the repo identity for the current invocation:
//
//   - If flagValue is non-empty, parse it as `<owner>/<name>` and
//     return the resulting Identity (errors with RepoFlagInvalid on
//     malformed input).
//   - Otherwise, fall back to Read.
func Resolve(ctx context.Context, flagValue string) (Identity, error) {
	if flagValue != "" {
		return ParseIdentity(flagValue)
	}
	return Read(ctx)
}
