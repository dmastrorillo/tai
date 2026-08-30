package cmd

import (
	"io"
	"os"
	"strings"
)

// isTruthyEnv reports whether the named environment variable holds a
// case-insensitive truthy value. The accepted set is:
//
//	1, true, yes, on, y, t
//
// Everything else — including unset, empty, `0`, `false`, `no`, `off`,
// `n`, `f`, and any unrecognised string — returns false. Future
// env-var toggles SHOULD reuse it so the accepted spellings stay
// consistent across verbs.
func isTruthyEnv(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on", "y", "t":
		return true
	}
	return false
}

// stdinIsTTY reports whether reader is an *os.File backed by a
// terminal device. urfave/cli wires c.Reader to os.Stdin in production
// and to a strings.Reader in tests; only the *os.File path can be a
// TTY.
//
// The mask `ModeDevice|ModeCharDevice` is the canonical stdlib idiom
// for "this file descriptor is a terminal". Using just ModeCharDevice
// can produce false positives on certain platforms where character
// devices that are not terminals also set that bit; requiring both
// rules them out.
func stdinIsTTY(reader io.Reader) bool {
	f, ok := reader.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	const ttyMask = os.ModeDevice | os.ModeCharDevice
	return fi.Mode()&ttyMask == ttyMask
}
