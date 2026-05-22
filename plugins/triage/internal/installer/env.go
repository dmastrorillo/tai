// This file holds environment-variable helpers shared between the
// install and uninstall code paths. The single helper today is
// IsTruthyEnv, kept here so future env-var toggles have an obvious
// home and reuse the same case-insensitive value table.

package installer

import (
	"os"
	"strings"
)

// IsTruthyEnv reports whether the named environment variable is set to
// a case-insensitive truthy value. The accepted set is:
//
//	1, true, yes, on, y, t
//
// Everything else — including unset, empty, `0`, `false`, `no`, `off`,
// `n`, `f`, and any unrecognised string — returns false. The helper
// lives here so install and uninstall share one definition; future
// env-var toggles SHOULD reuse it for consistency.
func IsTruthyEnv(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on", "y", "t":
		return true
	}
	return false
}
