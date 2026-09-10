package plugins

import (
	"os"
	"path/filepath"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// stateKeepPrefix names the temporary wrapper a plugin's state is
// parked in. It appears in the error a failed restore returns, which
// is the user's only route back to the data, so it is deliberately
// recognisable.
const stateKeepPrefix = "tai-state-keep-"

// parkedState is a plugin's runtime state held aside while its
// install directory is replaced or removed.
//
// A plugin keeps its state under `<dataDir>/plugins/<name>/state/` —
// the path the wire contract tells plugin authors to use — which sits
// inside the directory both install and remove destroy. For triage
// that directory holds the SQLite database with every imported review
// comment and every triage decision made against it: nothing in the
// release tarball can reconstruct it.
//
// `state/` is the only path either flow preserves. Everything else
// under the install directory belongs to the tarball and is replaced
// wholesale, so a stale binary or asset can never survive an update.
type parkedState struct {
	// statePath is where the state belongs and where restore puts it
	// back.
	statePath string
	// parked is where it currently sits, or "" when the plugin had no
	// state to move.
	parked string
}

// parkPluginState moves `<installDir>/state` aside so the caller can
// destroy and rebuild installDir.
//
// The wrapper is created as a sibling of installDir, keeping the move
// on one device so it cannot fail part-way across a filesystem
// boundary.
//
// A plugin with no state yet — a first install — parks nothing and
// yields a value whose restore is a no-op. An entry at `state` that
// is not a directory is out of contract (the wire contract specifies
// `state/`) and is deliberately not preserved: it belongs to the
// tarball's namespace and is replaced with everything else.
func parkPluginState(installDir string) (*parkedState, error) {
	p := &parkedState{statePath: filepath.Join(installDir, "state")}

	info, err := os.Stat(p.statePath)
	if err != nil || !info.IsDir() {
		return p, nil
	}

	tmp, err := os.MkdirTemp(filepath.Dir(installDir), stateKeepPrefix)
	if err != nil {
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"create state-keep directory beside %s", installDir)
	}
	parked := filepath.Join(tmp, "state")
	if err := os.Rename(p.statePath, parked); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"park plugin state %s", p.statePath)
	}
	p.parked = parked
	return p, nil
}

// held reports whether any state was actually parked.
func (p *parkedState) held() bool { return p.parked != "" }

// restore moves the state back to where it belongs, recreating the
// install directory if the caller's operation removed it. A no-op
// when nothing was parked, so callers may invoke it unconditionally —
// and MUST, on every exit path after a successful park, or the state
// is left in a wrapper directory nothing references.
//
// On failure the parked copy is the only one left, so it is kept and
// named in the error rather than cleaned up.
func (p *parkedState) restore() error {
	if p.parked == "" {
		return nil
	}
	installDir := filepath.Dir(p.statePath)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"recreate %s for state restore", installDir)
	}
	if err := os.Rename(p.parked, p.statePath); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"restore plugin state to %s", p.statePath).
			WithHelp(
				"the plugin's runtime state is parked at "+p.parked,
				"it is the only surviving copy — move it back to "+p.statePath+" by hand before re-running",
			)
	}
	_ = os.RemoveAll(filepath.Dir(p.parked))
	p.parked = ""
	return nil
}
