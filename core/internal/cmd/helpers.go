package cmd

import (
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// requireArgs returns the positional arguments for verb when exactly
// n were supplied, or a MISSING_ARG error naming the usage shape.
// Single source of the "requires exactly N arguments" contract every
// verb file used to hand-roll.
func requireArgs(c *cli.Command, verb, argSpec string, n int, help ...string) ([]string, error) {
	args := c.Args().Slice()
	if len(args) != n {
		return nil, errcode.Newf(errcode.MissingArg,
			"%s requires exactly %s: %s", verb, countNoun(n), argSpec).
			WithHelp(help...)
	}
	return args, nil
}

// requireOneArg is the single-argument convenience over requireArgs —
// the shape all but one verb needs.
func requireOneArg(c *cli.Command, verb, argName string, help ...string) (string, error) {
	args, err := requireArgs(c, verb, argName, 1, help...)
	if err != nil {
		return "", err
	}
	return args[0], nil
}

// countNoun spells the argument count the way the original inline
// messages did ("one argument", "two arguments").
func countNoun(n int) string {
	switch n {
	case 1:
		return "one argument"
	case 2:
		return "two arguments"
	default:
		return fmt.Sprintf("%d arguments", n)
	}
}

// resolveConfigPath wraps config.ResolvePath with the INTERNAL_ERROR
// mapping every verb applies — one place to change if the wrapping
// ever grows help text.
func resolveConfigPath() (string, error) {
	path, err := config.ResolvePath()
	if err != nil {
		return "", errcode.Wrap(errcode.InternalError, err, err.Error())
	}
	return path, nil
}

// loadEffectiveConfig loads tai's config, returning a non-nil
// pointer even when no file exists on disk. The shape shared by the
// plugin verbs, plugin dispatch, and any caller that only reads the
// effective config (verbs that also save go through loadOrEmpty with
// an explicit path instead).
func loadEffectiveConfig() (*config.File, error) {
	path, err := resolveConfigPath()
	if err != nil {
		return nil, err
	}
	return loadOrEmpty(path)
}
