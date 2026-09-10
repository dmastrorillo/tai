// Third-party consent for `<clone>/plugins.yml`.
//
// The plugin entries in a source repo's plugins.yml can point
// anywhere, and `tai sync` installs them without the user naming
// them. This is the gate that stops a repo owner adding somebody
// else's binary to a developer's machine silently.
//
// Consent is recorded against the exact file the user agreed to (see
// plugins.TrustStore), so an unchanged plugins.yml never asks twice
// and any edit asks again.
//
// Spec: openspec/specs/plugin-host/spec.md
// §"`plugins.yml` third-party trust cache for `tai sync`".

package sync

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// confirmThirdPartyPlugins gates the auto-install phase on the user
// having agreed to this plugins.yml. It returns nil when there is
// nothing third-party to agree to, when the file matches the recorded
// consent, or when the user agrees now — in which case the consent is
// recorded before any plugin is installed.
//
// interactive says whether there is a person on the other end of
// opts.Stdin. Without one, an unanswered prompt would hang a sync in
// CI forever, so the refusal is immediate and names the flag that
// unblocks it.
func confirmThirdPartyPlugins(entries []pluginsYAMLEntry, raw []byte, repoURL, dataDir string, opts Options, interactive bool) error {
	sources := thirdPartySources(entries)
	if len(sources) == 0 {
		return nil
	}

	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])

	store, err := plugins.LoadTrust(dataDir)
	if err != nil {
		return err
	}
	if agreed, ok := store.Get(repoURL); ok && agreed == digest {
		return nil
	}

	if !opts.TrustThirdParty {
		if !interactive {
			return unconfirmedYAMLError(sources)
		}
		_, _ = fmt.Fprintf(opts.Stderr,
			"Source repo plugins.yml lists third-party plugins:\n%s\nThird-party plugins run arbitrary code on your machine. Continue? [y/N] ",
			"  "+strings.Join(sources, "\n  "))
		line, _ := bufio.NewReader(ensureStdin(opts.Stdin)).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
		default:
			return unconfirmedYAMLError(sources)
		}
	}

	// Recorded before the installs run: a fetch that fails afterwards
	// does not un-say what the user just said, and re-running the
	// sync must not ask again.
	store.Put(repoURL, digest)
	return plugins.SaveTrust(dataDir, store)
}

// thirdPartySources returns the `<host>/<repo>` label of every entry
// that resolves to a source outside the built-in registry.
//
// An entry that resolves to nothing at all — no source spec and no
// registry hit — is not third-party, it is broken. Leaving it out
// keeps the consent prompt about trust and lets the install surface
// PLUGIN_UNKNOWN with its own, more useful message.
func thirdPartySources(entries []pluginsYAMLEntry) []string {
	out := []string{}
	for _, e := range entries {
		src := plugins.ParseSource(e.Source)
		if src.Empty() {
			registered, ok := plugins.Lookup(e.Name)
			if !ok {
				continue
			}
			src = registered
		}
		if plugins.IsBuiltin(src) {
			continue
		}
		out = append(out, e.Name+" ("+src.Host+"/"+src.Repo+")")
	}
	return out
}

func unconfirmedYAMLError(sources []string) error {
	return errcode.Newf(errcode.PluginThirdpartyUnconfirmed,
		"the source repo's plugins.yml lists third-party plugins that need confirmation: %s",
		strings.Join(sources, ", ")).
		WithHelp(
			"re-run with `--trust-third-party` to confirm you trust these sources",
			"or run `tai sync` in a terminal to be asked interactively",
			"or remove the entries from the source repo's plugins.yml",
		)
}

// ensureStdin returns a non-nil reader so the prompt path never
// dereferences a nil Stdin.
func ensureStdin(r io.Reader) io.Reader {
	if r == nil {
		return strings.NewReader("")
	}
	return r
}
