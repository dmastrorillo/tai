// Package installtest provides shared test helpers for code that
// exercises installer.Install and installer.Uninstall.
//
// The package is regular (non-_test) Go because both end-to-end command
// tests (`internal/cmd/*_test.go`) and the installer's own engine tests
// (`internal/installer/run_test.go`) need the same fake-Bundle
// implementation. Sharing a single source-of-truth fake keeps the
// `installer.Bundle` interface honest: when the interface changes there
// is exactly one consumer to update.
//
// Tests outside the installer family should not import this package.
package installtest

import (
	"fmt"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdframework"
	"github.com/dmastrorillo/tai/plugins/triage/internal/installer"
)

// FakeBundle is an in-memory Bundle implementation. Tests construct one
// directly (or via NewSingleVerb) and inject it through
// installer.Options.Bundle / cmd.WithBundle.
type FakeBundle struct {
	VerbsList []string
	Sources   map[string][]byte
	Ledgers   map[string][]string
}

// Compile-time assertion that FakeBundle satisfies installer.Bundle.
var _ installer.Bundle = (*FakeBundle)(nil)

// Verbs returns the configured list of verbs.
func (f *FakeBundle) Verbs() []string { return f.VerbsList }

// Source returns the bundled `<verb>.md` source bytes, or an error for
// an unknown verb.
func (f *FakeBundle) Source(verb string) ([]byte, error) {
	b, ok := f.Sources[verb]
	if !ok {
		return nil, fmt.Errorf("installtest: unknown verb %q", verb)
	}
	return b, nil
}

// Ledger returns the cumulative hash history for verb (empty slice for
// unknown verbs, mirroring the production behaviour).
func (f *FakeBundle) Ledger(verb string) ([]string, error) {
	return f.Ledgers[verb], nil
}

// ProbeSrc is the canonical bundled-command markdown reused by every
// installer test. The body hash is fixed and reproducible — derivable
// via cmdframework.HashBody on the body portion.
const ProbeSrc = `---
name: "TAI: Probe"
description: "Probe."
category: "Workflow"
tags: [probe]
version: 1
content_hash: "sha256:0000000000000000000000000000000000000000000000000000000000000000"
---
body of probe
`

// NewSingleVerb returns a FakeBundle pre-populated with one verb
// ("probe") whose current body matches ProbeSrc and whose ledger
// contains exactly that body's hash.
func NewSingleVerb() *FakeBundle {
	body, err := cmdframework.Body([]byte(ProbeSrc))
	if err != nil {
		// Programmer error in this package, not in the caller.
		panic(fmt.Sprintf("installtest: ProbeSrc fails to parse: %v", err))
	}
	hash := cmdframework.HashBody(body)
	return &FakeBundle{
		VerbsList: []string{"probe"},
		Sources:   map[string][]byte{"probe": []byte(ProbeSrc)},
		Ledgers:   map[string][]string{"probe": {hash}},
	}
}
