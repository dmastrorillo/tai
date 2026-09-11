// The `plugins.yml` third-party trust cache.
//
// A source repo can list plugins for `tai sync` to install, and those
// entries can point anywhere. Asking on every sync would train the
// user to agree without reading; never asking would let a repo owner
// add a third-party binary silently. The compromise recorded here is
// consent to an exact file: the sha256 of the `plugins.yml` the user
// agreed to, keyed on the repo it came from. Change the file, and the
// question is asked again.
//
// Spec: openspec/specs/plugin-host/spec.md
// §"`plugins.yml` third-party trust cache for `tai sync`".

package plugins

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// TrustStore is the on-disk record at
// `<TAI_DATA_DIR>/state/trust.json`. Like plugins.json its schema is
// append-only: fields may be added, never renamed or removed.
type TrustStore struct {
	Trust []TrustEntry `json:"trust"`
}

// TrustEntry is one repo's agreed-to `plugins.yml`.
type TrustEntry struct {
	RepoURL string `json:"repo-url"`
	SHA256  string `json:"plugins-yml-sha256"`
}

func trustPath(dataDir string) string {
	return filepath.Join(dataDir, "state", "trust.json")
}

// LoadTrust reads the trust store. A missing file is an empty store,
// not an error — nobody has agreed to anything yet.
func LoadTrust(dataDir string) (*TrustStore, error) {
	body, err := os.ReadFile(trustPath(dataDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &TrustStore{}, nil
		}
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"read %s", trustPath(dataDir))
	}
	var store TrustStore
	if err := json.Unmarshal(body, &store); err != nil {
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"parse %s", trustPath(dataDir)).
			WithHelp("delete the file to start over; you will be asked to confirm again")
	}
	return &store, nil
}

// SaveTrust writes the store back.
func SaveTrust(dataDir string, store *TrustStore) error {
	p := trustPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"create %s", filepath.Dir(p))
	}
	body, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "marshal trust store")
	}
	body = append(body, '\n')
	if err := os.WriteFile(p, body, 0o644); err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "write %s", p)
	}
	return nil
}

// Get returns the agreed-to hash for repoURL.
func (s *TrustStore) Get(repoURL string) (string, bool) {
	for _, e := range s.Trust {
		if e.RepoURL == repoURL {
			return e.SHA256, true
		}
	}
	return "", false
}

// Put records repoURL's agreed-to hash, replacing any prior one.
//
// Entries for other repo URLs are left alone. A user who switches
// `repo-url` back and forth is not asked again for a repo they have
// already agreed to, and cleaning out stale rows is their affair.
func (s *TrustStore) Put(repoURL, sha string) {
	for i := range s.Trust {
		if s.Trust[i].RepoURL == repoURL {
			s.Trust[i].SHA256 = sha
			return
		}
	}
	s.Trust = append(s.Trust, TrustEntry{RepoURL: repoURL, SHA256: sha})
}
