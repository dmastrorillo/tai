package cliout

// The one yes/no prompt parser. Every consent surface reads its answer
// through here so a change to what counts as a yes — a new accepted
// spelling, a different default — lands once rather than once per
// call site. The authoritative package doc lives in cliout.go.

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// ConfirmYesNo reads one line from stdin and reports whether it is an
// explicit yes: `y` or `yes`, any case, surrounding whitespace
// ignored.
//
// Everything else is a no, including an empty line and an empty
// stdin. The default is deliberately the safe one — a bare Enter is
// the answer a `[y/N]` prompt promises to read as N, and an absent
// answer must never be upgraded into agreement.
//
// A read failure returns false with the cause wrapped. Callers get
// both halves of what they need: the deny, and something to report.
// The error is NOT the same as a no — a caller gating a destructive or
// security-relevant action should surface it rather than treating it
// as a plain refusal.
//
// Callers are responsible for printing the question first; this
// function only reads the answer.
func ConfirmYesNo(stdin io.Reader) (bool, error) {
	if stdin == nil {
		return false, nil
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	// EOF is not a failure: it is stdin saying there is no more to
	// read, and any bytes before it are still a valid answer.
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}
