package board

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/pkg/datadir"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// ServeChildEnv marks the process as the board's server rather than the
// command that launched it. It is an environment variable and not a
// command-line argument for the same reason the browser is handed a
// secret-free landing path: argv is readable by every other local user
// on macOS and most Linux configurations.
const ServeChildEnv = "TRIAGE_BOARD_SERVE"

// IsServeChild reports whether this process is the detached server.
func IsServeChild() bool { return os.Getenv(ServeChildEnv) == "1" }

// readyTimeout bounds how long the launching command waits for the
// server process to report itself when its caller supplies no earlier
// deadline. Binding a loopback socket and rendering nothing takes
// milliseconds; a server that has not spoken in ten seconds is not
// about to.
const readyTimeout = 10 * time.Second

// The server process reports itself on a single line of its stdout,
// which is a pipe held by the command that launched it. Two forms:
//
//	ready <board-url>
//	error <message>
//
// A line rather than a shared file because the pipe is private to the
// two processes: the board URL carries the path prefix that keeps the
// briefing unreadable to anything else on the machine, and a file would
// publish it to every process that can stat the directory.
const (
	readyPrefix = "ready "
	errorPrefix = "error "
)

// FormatReady renders the line the server process writes to announce
// itself. Exported because the announcement is made through Serve's
// announce callback, which the command layer supplies.
func FormatReady(url string) string { return readyPrefix + url + "\n" }

// loopbackBindHelp is the remediation both halves of a bind failure
// offer. The child produces the failure and the launching command
// relays it, so a single run can surface either; stating it once keeps
// them from drifting into two different answers to one question.
func loopbackBindHelp() []string {
	return []string{
		"check whether a local firewall or sandbox blocks binding 127.0.0.1",
		"the board needs no outbound network access, only a loopback socket",
	}
}

// ─── the launching command ───────────────────────────────────────────

// spawn starts the board's server process and returns the pipe it
// reports itself on, plus the way to stop it if it never does.
//
// The seam is here rather than around the whole hand-off so that
// readReady — and with it the readiness-line format the two processes
// agree on — runs for real in every test that drives the command layer.
// A seam placed above it would let the format change on one side only
// and leave the suite green.
var spawn = spawnProcess

// SpawnForTesting replaces the server-process spawn for the lifetime of
// t. The testing.TB parameter is the guard: a production binary that
// imports `testing` deliberately is a code-review red flag.
func SpawnForTesting(t testing.TB, fn func(briefing []byte) (io.Reader, func(), error)) {
	t.Helper()
	prev := spawn
	spawn = fn
	t.Cleanup(func() { spawn = prev })
}

// Launch starts the board and returns once it is serving. The developer
// works the board for as long as they need to and their decisions come
// back through the intents artifact, so the command that calls this has
// nothing left to wait for.
//
// The announcement follows the server reporting itself, never precedes
// it: a URL on stdout is the caller's only handle on the board, so one
// that nothing is serving sends them looking for a failure that left no
// other trace.
func Launch(ctx context.Context, briefing []byte, announce func(url string)) error {
	url, err := spawnServer(ctx, briefing)
	if err != nil {
		// The cause is repeated into the message because the error
		// template renders Msg and nothing else, and the cause here is
		// the whole diagnosis — a bind refused by a sandbox and a
		// binary that could not be re-executed are different problems
		// with different fixes.
		return errcode.Wrapf(errcode.TriageBoardUnavailable, err,
			"starting the board's server process: %s", err).
			WithHelp(loopbackBindHelp()...)
	}
	announce(url)
	return nil
}

// spawnServer starts the server process and reads the one line it
// writes back.
func spawnServer(ctx context.Context, briefing []byte) (string, error) {
	ready, stop, err := spawn(briefing)
	if err != nil {
		return "", err
	}
	url, err := readReady(ctx, ready)
	if err != nil {
		stop()
		return "", err
	}
	return url, nil
}

// spawnProcess re-execs this binary as the board's server in a session
// of its own and feeds it the briefing on stdin.
//
// It is deliberately not `exec.CommandContext`: the point of the process
// is to outlive this one, so a context that ends with this command must
// not end the board.
func spawnProcess(briefing []byte) (io.Reader, func(), error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("locating this binary: %w", err)
	}

	// An os.Pipe rather than cmd.StdoutPipe: the latter is closed by
	// cmd.Wait, and nothing here ever waits on a process whose whole
	// job is to still be running when this one is gone.
	readyR, readyW, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("opening the readiness pipe: %w", err)
	}

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		_ = readyR.Close()
		_ = readyW.Close()
		return nil, nil, fmt.Errorf("opening the briefing pipe: %w", err)
	}
	defer func() { _ = stdinR.Close() }()

	errLog := openServerLog()
	defer func() { _ = errLog.Close() }()

	cmd := exec.Command(exe, "board", "-")
	cmd.Env = append(os.Environ(), ServeChildEnv+"=1")
	cmd.Stdin = stdinR
	cmd.Stdout = readyW
	cmd.Stderr = errLog
	cmd.SysProcAttr = detachAttr()

	if err := cmd.Start(); err != nil {
		_ = readyR.Close()
		_ = readyW.Close()
		_ = stdinW.Close()
		return nil, nil, fmt.Errorf("starting %s: %w", exe, err)
	}
	// This process's copy of the child's end is closed so the reader
	// sees EOF when the child exits rather than blocking on a writer
	// only this process still holds.
	_ = readyW.Close()

	// A briefing may run to megabytes and a pipe holds far less, so the
	// write has to proceed while the child is draining it. Its error is
	// kept rather than dropped: a briefing that never arrived and a
	// child that never started both end as a closed pipe here, and only
	// this error tells them apart.
	writeErr := make(chan error, 1)
	go func() {
		_, err := stdinW.Write(briefing)
		_ = stdinW.Close()
		writeErr <- err
	}()

	stop := func() {
		_ = cmd.Process.Kill()
		_ = readyR.Close()
	}
	return readyReader{r: readyR, writeErr: writeErr}, stop, nil
}

// readyReader turns a silent end-of-pipe into the reason the briefing
// never landed, when that is what ended it.
type readyReader struct {
	r        io.Reader
	writeErr <-chan error
}

func (rr readyReader) Read(p []byte) (int, error) {
	n, err := rr.r.Read(p)
	if err == nil || !errors.Is(err, io.EOF) {
		return n, err
	}
	select {
	case werr := <-rr.writeErr:
		if werr != nil {
			return n, fmt.Errorf("the briefing never reached the server process: %w", werr)
		}
	default:
	}
	return n, err
}

// openServerLog returns the file the server process's stderr is pointed
// at. The board's page render logs there on failure (see handlePage),
// and that log has nowhere else to go: the process is detached, so it
// has no terminal, and the readiness pipe is closed by the time it is
// serving.
//
// A log that cannot be opened must never stop a board launching, so the
// fallback is the null device and the launch proceeds.
func openServerLog() *os.File {
	path, err := ServerLogPath()
	if err == nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err == nil {
				return f
			}
		}
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return devNull
}

// ServerLogPath is where a detached board writes what it cannot say.
func ServerLogPath() (string, error) {
	base, err := datadir.Resolve()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "plugins", "triage", "state", "board.log"), nil
}

// readReady reads the server process's single line of self-report. It
// returns when the line arrives, when ctx ends, or when readyTimeout
// expires — whichever is first.
func readReady(ctx context.Context, r io.Reader) (string, error) {
	type result struct {
		line string
		err  error
	}
	lines := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(r).ReadString('\n')
		lines <- result{line: line, err: err}
	}()

	select {
	case got := <-lines:
		line := strings.TrimRight(got.line, "\r\n")
		switch {
		case strings.HasPrefix(line, readyPrefix):
			return strings.TrimPrefix(line, readyPrefix), nil
		case strings.HasPrefix(line, errorPrefix):
			return "", errors.New(strings.TrimPrefix(line, errorPrefix))
		case got.err != nil && !errors.Is(got.err, io.EOF):
			return "", fmt.Errorf("reading the server process's readiness line: %w", got.err)
		case got.err != nil:
			return "", errors.New("the server process exited before it began serving")
		default:
			return "", fmt.Errorf("the server process reported %q, which is not a readiness line", line)
		}
	case <-ctx.Done():
		return "", fmt.Errorf("waiting for the server process: %w", ctx.Err())
	case <-time.After(readyTimeout):
		return "", fmt.Errorf("the server process did not report itself within %s", readyTimeout)
	}
}

// ─── the server process ──────────────────────────────────────────────

// ServeDetached is the body of the board's server process: it serves the
// briefing until the developer submits and reports itself on w, which is
// the pipe the launching command reads before it announces the URL.
//
// A bind failure is reported on the same pipe rather than left to this
// process's stderr, which no terminal is attached to. The launching
// command is the only one with a terminal, so it is the only one that
// can tell anybody.
func ServeDetached(ctx context.Context, b Briefing, w io.Writer) ([]IntentEntry, error) {
	srv, err := NewServer(b)
	if err != nil {
		reportFailure(w, err)
		return nil, err
	}
	entries, err := srv.Serve(ctx, func(url string) {
		_, _ = io.WriteString(w, FormatReady(url))
	})
	if err != nil {
		reportFailure(w, err)
		return nil, err
	}
	return entries, nil
}

// reportFailure sends one line back up the readiness pipe. The message is
// flattened to a single line because the pipe carries exactly one.
func reportFailure(w io.Writer, err error) {
	msg := strings.Join(strings.Fields(err.Error()), " ")
	_, _ = io.WriteString(w, errorPrefix+msg+"\n")
}
