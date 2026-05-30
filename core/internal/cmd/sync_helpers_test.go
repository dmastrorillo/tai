package cmd_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/cmd"
	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/cliexec"
	"github.com/dmastrorillo/tai/pkg/cliout"
)

// runRootStdin drives cmd.NewRoot like runRoot but feeds the supplied
// string as stdin. Used by sync tests that exercise the overwrite/prune
// prompt's stdin read.
//
// Not tied to a TC-ID — it's a test helper. Kept in a separate file so
// the runRoot helper in root_test.go stays minimal.
func runRootStdin(t *testing.T, stdin string, argv ...string) runResult {
	t.Helper()

	var stdout, stderr bytes.Buffer
	root := cmd.NewRoot()
	wireStreamsWithStdin(root, &stdout, &stderr, strings.NewReader(stdin))

	fullArgs := append([]string{"tai"}, argv...)
	err := cliexec.Run(context.Background(), root, fullArgs)
	if err != nil {
		cliout.WriteError(&stderr, err)
	}
	return runResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: exitCodeFor(err),
		err:      err,
	}
}

// wireStreamsWithStdin is wireStreams with a configurable stdin
// reader instead of the empty string the runRoot helper assumes.
func wireStreamsWithStdin(c *cli.Command, out, errOut *bytes.Buffer, in *strings.Reader) {
	c.Writer = out
	c.ErrWriter = errOut
	c.Reader = in
	for _, child := range c.Commands {
		wireStreamsWithStdin(child, out, errOut, in)
	}
}

// pollDirect runs sync.Poll synchronously against the current env's
// config. Used by TC-SYNC-014/015/016/017 to avoid the
// fire-and-forget shape of the production Schedule() goroutine.
//
// Not tied to a TC-ID — test fixture helper.
func pollDirect(t *testing.T, _ /*url*/, dataDir string) {
	t.Helper()
	cfgPath, err := config.ResolvePath()
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	// Poll's error is intentionally swallowed in production. In the
	// tests we let it through for diagnostic logging only — we still
	// assert on the state file as the user-observable contract.
	if pollErr := sync.Poll(context.Background(), cfg, dataDir); pollErr != nil {
		t.Logf("sync.Poll returned (non-fatal): %v", pollErr)
	}
}
