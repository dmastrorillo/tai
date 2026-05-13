// This file holds the per-verb hash-history access contract. The
// concrete data source (checked-in commands/<verb>.ledger.json files
// embedded via //go:embed) is populated by add-install-command; the
// foundation only specifies the function signature so install-time
// classifiers can compile and link against it today.

package cmdframework

// Ledger returns the cumulative history of content_hash values ever
// shipped for the named verb's slash-command markdown. Order is
// oldest-first; the LAST element MUST equal the hash of the current
// build's commands/<verb>.md (a property the build-time test enforces).
//
// The foundation contract is "this function exists and returns a slice".
// The concrete data source — checked-in commands/<verb>.ledger.json
// files embedded via //go:embed — is owned by add-install-command. Until
// that proposal lands, the function returns an empty slice for every
// verb, signalling "no history yet" to install-time classifiers.
//
// Future install code MUST tolerate an empty Ledger return (treat every
// file on disk as user-modified). The foundation establishes the
// contract; install populates the data.
func Ledger(verb string) []string {
	_ = verb
	return nil
}
