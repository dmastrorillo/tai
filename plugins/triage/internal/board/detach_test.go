package board

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// TestBoard_TCBRD012_the_detached_server_announces_and_returns_on_submit
// drives the body of the board's server process: the readiness line the
// launching command parses, a real HTTP round trip over the socket that
// line names, and the return that follows a submit.
//
// The launching command's side of the same protocol is
// TestBoard_TCBRD033_returns_without_waiting_for_a_submit in the cmd
// tree; the two meeting in a real pair of processes is TC-BRD-035.
//
// NoBrowserForTesting is mandatory here, not incidental. A test run must
// never open a real window.
func TestBoard_TCBRD012_the_detached_server_announces_and_returns_on_submit(t *testing.T) {
	NoBrowserForTesting(t)

	b := Briefing{
		Repo: "acme/app", Scope: prScope(142),
		Comments: []Comment{fullComment(1, ""), fullComment(2, "")},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	readyR, readyW := io.Pipe()
	done := make(chan []IntentEntry, 1)
	errs := make(chan error, 1)
	go func() {
		entries, err := ServeDetached(ctx, b, readyW)
		if err != nil {
			errs <- err
			return
		}
		done <- entries
	}()

	line, err := bufio.NewReader(readyR).ReadString('\n')
	if err != nil {
		t.Fatalf("reading the readiness line: %v", err)
	}
	line = strings.TrimRight(line, "\n")
	if !strings.HasPrefix(line, "ready ") {
		t.Fatalf("the server must report itself as a readiness line, got %q", line)
	}
	url := strings.TrimPrefix(line, "ready ")
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Errorf("the board must bind loopback on a kernel-assigned port, got %q", url)
	}
	if !strings.Contains(url, "/b/") {
		t.Errorf("the announced URL must carry the board's path prefix, got %q", url)
	}

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET the announced URL: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET the board: want 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "src/api/auth.ts:15-29") {
		t.Error("the served page did not carry the briefed comment")
	}

	post, err := http.Post(url+"submit", "application/json",
		strings.NewReader(`[{"id":1,"intent":"accept","note":"via the real socket"}]`))
	if err != nil {
		t.Fatalf("POST submit: %v", err)
	}
	_ = post.Body.Close()

	select {
	case entries := <-done:
		if len(entries) != 2 {
			t.Fatalf("every briefed comment gets an entry, got %d", len(entries))
		}
		byID := map[int]IntentEntry{}
		for _, e := range entries {
			byID[e.ID] = e
		}
		if byID[1].Intent != IntentAccept || byID[1].Note != "via the real socket" {
			t.Errorf("comment 1 round-tripped wrong: %+v", byID[1])
		}
		if byID[2].Intent != IntentUnanswered {
			t.Errorf("an untouched comment must come back unanswered, got %q", byID[2].Intent)
		}
	case err := <-errs:
		t.Fatalf("the server returned an error after submit: %v", err)
	case <-ctx.Done():
		t.Fatal("the server did not return after the board was submitted")
	}
}

// TestBoard_TCBRD013_a_listener_that_cannot_bind_is_reported_up_the_pipe
// covers the one failure the server process can suffer after the
// briefing has already been validated. Its stderr is /dev/null, so the
// readiness pipe is the only way the reason reaches anybody.
func TestBoard_TCBRD013_a_listener_that_cannot_bind_is_reported_up_the_pipe(t *testing.T) {
	ListenFailureForTesting(t, errors.New("bind: permission denied"))

	var reported bytes.Buffer
	_, err := ServeDetached(context.Background(), Briefing{
		Repo: "acme/app", Scope: prScope(1),
		Comments: []Comment{fullComment(1, "")},
	}, &reported)

	var coded *errcode.Error
	if !errors.As(err, &coded) {
		t.Fatalf("want a coded error, got %v", err)
	}
	if coded.Code != errcode.TriageBoardUnavailable {
		t.Errorf("want TRIAGE_BOARD_UNAVAILABLE, got %s", coded.Code)
	}
	line := strings.TrimRight(reported.String(), "\n")
	if !strings.HasPrefix(line, "error ") {
		t.Fatalf("the failure must be reported up the pipe, got %q", line)
	}
	if !strings.Contains(line, "bind: permission denied") {
		t.Errorf("the reported line must carry the reason, got %q", line)
	}
	if strings.Contains(line, "\n") {
		t.Errorf("the pipe carries exactly one line, got %q", line)
	}
}
