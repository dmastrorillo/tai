// This file is an internal-package test (package cmdframework, not
// cmdframework_test) so it can drive ledgerFromFS — the unexported core
// of LedgerStrict — with synthetic filesystems. The real BundleFS
// embed.FS has no corrupt files by construction, so these branches
// would otherwise be untestable.

package cmdframework

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/dmastrorillo/tai/internal/errcode"
)

// assertCorrupt is a small helper: confirm err is a
// *errcode.Error{Code: InstallLedgerCorrupt}.
func assertCorrupt(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected InstallLedgerCorrupt error, got nil")
	}
	var e *errcode.Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if e.Code != errcode.InstallLedgerCorrupt {
		t.Fatalf("expected code %s, got %s", errcode.InstallLedgerCorrupt, e.Code)
	}
}

// TestLedgerFromFS_corrupt_json_surfaces_install_ledger_corrupt: a
// ledger file whose bytes do not parse as JSON returns the
// InstallLedgerCorrupt code, per spec scenario "Corrupt ledger surfaces
// at runtime".
func TestLedgerFromFS_corrupt_json_surfaces_install_ledger_corrupt(t *testing.T) {
	fsys := fstest.MapFS{
		"commands/probe.ledger.json": &fstest.MapFile{Data: []byte("not valid json {{{")},
	}
	_, err := ledgerFromFS(fsys, "probe")
	assertCorrupt(t, err)
}

// TestLedgerFromFS_malformed_hash_entry_surfaces_install_ledger_corrupt:
// a syntactically-JSON ledger whose entries don't match the
// sha256:<64-lower-hex> shape returns InstallLedgerCorrupt.
func TestLedgerFromFS_malformed_hash_entry_surfaces_install_ledger_corrupt(t *testing.T) {
	fsys := fstest.MapFS{
		"commands/probe.ledger.json": &fstest.MapFile{
			Data: []byte(`["sha256:NOTHEX", "sha256:0000000000000000000000000000000000000000000000000000000000000001"]`),
		},
	}
	_, err := ledgerFromFS(fsys, "probe")
	assertCorrupt(t, err)
}

// TestLedgerFromFS_uppercase_hex_rejected: hashes must be lower-case
// hex per the regex `^sha256:[0-9a-f]{64}$`; an upper-case entry is
// rejected as malformed.
func TestLedgerFromFS_uppercase_hex_rejected(t *testing.T) {
	fsys := fstest.MapFS{
		"commands/probe.ledger.json": &fstest.MapFile{
			Data: []byte(`["sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"]`),
		},
	}
	_, err := ledgerFromFS(fsys, "probe")
	assertCorrupt(t, err)
}

// TestLedgerFromFS_missing_returns_empty_no_error: when the verb has no
// ledger file, ledgerFromFS returns an empty slice and no error — the
// "unknown verb / no history yet" shape callers rely on.
func TestLedgerFromFS_missing_returns_empty_no_error(t *testing.T) {
	fsys := fstest.MapFS{}
	hashes, err := ledgerFromFS(fsys, "probe")
	if err != nil {
		t.Fatalf("expected nil error for missing ledger, got %v", err)
	}
	if len(hashes) != 0 {
		t.Fatalf("expected empty slice, got %v", hashes)
	}
}

// TestLedgerFromFS_well_formed_round_trips: a well-formed ledger parses
// successfully and the returned slice preserves order.
func TestLedgerFromFS_well_formed_round_trips(t *testing.T) {
	in := []string{
		"sha256:0000000000000000000000000000000000000000000000000000000000000001",
		"sha256:0000000000000000000000000000000000000000000000000000000000000002",
	}
	fsys := fstest.MapFS{
		"commands/probe.ledger.json": &fstest.MapFile{
			Data: []byte(`["` + in[0] + `","` + in[1] + `"]`),
		},
	}
	got, err := ledgerFromFS(fsys, "probe")
	if err != nil {
		t.Fatalf("ledgerFromFS: %v", err)
	}
	if len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
		t.Fatalf("ledger round-trip mismatch\nwant: %v\ngot:  %v", in, got)
	}
}
