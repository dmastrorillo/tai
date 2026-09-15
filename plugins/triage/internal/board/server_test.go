package board

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func serverFor(t *testing.T, b Briefing) *Server {
	t.Helper()
	s, err := NewServer(b)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

// renderPage drives the board's own routing surface, so the prefix
// check and the template run exactly as they do when served.
func renderPage(t *testing.T, s *Server) string {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/b/"+s.prefix+"/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for the board path, got %d", rec.Code)
	}
	return rec.Body.String()
}

func fullComment(id int, batch string) Comment {
	c := comment(id, "critical", batch)
	c.RaisedBy = "coderabbit"
	c.Location = "src/api/auth.ts:15-29"
	c.Description = "execSync interpolates user input"
	c.Cause = "config passes both values into a shell string"
	c.WhyFix = "shell metacharacters execute"
	c.SuggestedFix = "use execFileSync with an argv slice"
	c.ConcernsIfSkipped = "arbitrary command execution on the build host"
	return c
}

func TestBoard_TCBRD010_refuses_a_request_without_the_path_prefix(t *testing.T) {
	s := serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(142),
		Comments: []Comment{fullComment(1, "")},
	})

	for _, path := range []string{"/", "/b/", "/b/0000000000000000/", "/index.html"} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: want 404, got %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "execSync interpolates") {
			t.Errorf("%s: the 404 body leaked briefing content", path)
		}
	}
}

func TestBoard_TCBRD014_renders_all_seven_presentation_fields(t *testing.T) {
	c := fullComment(7, "")
	body := renderPage(t, serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(142), Comments: []Comment{c},
	}))

	for name, want := range map[string]string{
		"raised_by":           c.RaisedBy,
		"location":            c.Location,
		"description":         c.Description,
		"cause":               c.Cause,
		"why_fix":             c.WhyFix,
		"suggested_fix":       c.SuggestedFix,
		"concerns_if_skipped": c.ConcernsIfSkipped,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%s missing from the served HTML; the board must show every "+
				"field the loop presents, or a bulk decision is made on less", name)
		}
	}
}

func TestBoard_TCBRD015_marks_an_investigation_authored_fix(t *testing.T) {
	c := fullComment(1, "")
	c.SuggestedFixOrigin = "investigation"
	body := renderPage(t, serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(1), Comments: []Comment{c},
	}))
	if !strings.Contains(body, "proposed by the investigation") {
		t.Error("a fix the investigation derived must be distinguishable from the reviewer's")
	}

	c.SuggestedFixOrigin = "reviewer"
	body = renderPage(t, serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(1), Comments: []Comment{c},
	}))
	if !strings.Contains(body, "the reviewer's proposal") {
		t.Error("a reviewer's fix must say so")
	}
}

func TestBoard_TCBRD016_groups_batch_members_with_both_control_levels(t *testing.T) {
	b := Briefing{
		Repo: "acme/app", Scope: prScope(1),
		Batches: []Batch{{BatchKey: "B1", Title: "Replace execSync with execFileSync"}},
		Comments: []Comment{
			fullComment(1, "B1"), fullComment(2, "B1"), fullComment(3, "B1"),
			fullComment(4, "B1"), fullComment(5, "B1"),
		},
	}
	body := renderPage(t, serverFor(t, b))

	if !strings.Contains(body, "Replace execSync with execFileSync") {
		t.Error("the batch title must be rendered")
	}
	if strings.Count(body, `<button type="button" data-batch-intent="accept"`) != 1 {
		t.Error("a batch needs exactly one batch-level accept control")
	}
	if got := strings.Count(body, `<button type="button" data-intent="accept"`); got != 5 {
		t.Errorf("each of the five members needs its own control, found %d", got)
	}
}

func TestBoard_TCBRD017_puts_batches_before_non_batched_comments(t *testing.T) {
	major := fullComment(1, "B1")
	major.Severity = "major"
	loose := fullComment(2, "")
	loose.Severity = "critical"

	body := renderPage(t, serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(1),
		Batches:  []Batch{{BatchKey: "B1", Title: "the batch"}},
		Comments: []Comment{loose, major},
	}))

	batchAt := strings.Index(body, "the batch")
	looseAt := strings.Index(body, `data-id="2"`)
	if batchAt == -1 || looseAt == -1 {
		t.Fatal("both the batch and the loose comment must render")
	}
	if batchAt > looseAt {
		t.Error("batches precede non-batched comments even when a loose comment " +
			"is more severe — the loop orders the same way")
	}
}

func TestBoard_TCBRD018_gives_every_comment_and_batch_a_note_input(t *testing.T) {
	body := renderPage(t, serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(1),
		Batches: []Batch{{BatchKey: "B1", Title: "the batch"}},
		Comments: []Comment{
			fullComment(1, "B1"), fullComment(2, "B1"), fullComment(3, "B1"),
			fullComment(4, ""), fullComment(5, ""),
		},
	}))

	if got := strings.Count(body, "data-note="); got != 5 {
		t.Errorf("want a note input on each of the 5 comments, found %d", got)
	}
	if got := strings.Count(body, "data-batch-note="); got != 1 {
		t.Errorf("want a note input on the batch, found %d", got)
	}
}

func TestBoard_TCBRD019_renders_and_submits_an_empty_briefing(t *testing.T) {
	s := serverFor(t, Briefing{Repo: "acme/app", Scope: prScope(1)})
	body := renderPage(t, s)
	if !strings.Contains(body, "nothing to decide") {
		t.Error("an empty briefing must say so rather than render a blank page")
	}

	entries := submit(t, s, `[]`)
	if len(entries) != 0 {
		t.Errorf("an empty briefing submits an empty intent set, got %+v", entries)
	}
}

// submit posts a decision set and returns the entries the server derived.
func submit(t *testing.T, s *Server, body string) []IntentEntry {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/b/"+s.prefix+"/submit", strings.NewReader(body))
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	select {
	case sub := <-s.submitted:
		return sub.entries
	default:
		t.Fatal("submit did not deliver a decision set")
		return nil
	}
}

func TestBoard_TCBRD020_submit_records_every_briefed_comment(t *testing.T) {
	s := serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(1),
		Comments: []Comment{fullComment(1, ""), fullComment(2, ""), fullComment(3, "")},
	})

	entries := submit(t, s, `[
	  {"id":1,"intent":"accept","note":""},
	  {"id":2,"intent":"dismiss","note":"covered by the sandbox"}
	]`)

	if len(entries) != 3 {
		t.Fatalf("every briefed comment gets an entry, got %d", len(entries))
	}
	byID := map[int]IntentEntry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	if byID[1].Intent != IntentAccept {
		t.Errorf("comment 1: want accept, got %q", byID[1].Intent)
	}
	if byID[2].Intent != IntentDismiss || byID[2].Note != "covered by the sandbox" {
		t.Errorf("comment 2: want dismiss with its note, got %+v", byID[2])
	}
	if byID[3].Intent != IntentUnanswered {
		t.Errorf("a comment the developer never touched is unanswered, got %q", byID[3].Intent)
	}
}

func TestBoard_TCBRD022_a_batch_split_is_recorded_per_member(t *testing.T) {
	s := serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(1),
		Batches: []Batch{{BatchKey: "B1", Title: "the batch"}},
		Comments: []Comment{
			fullComment(1, "B1"), fullComment(2, "B1"), fullComment(3, "B1"),
			fullComment(4, "B1"), fullComment(5, "B1"),
		},
	})

	entries := submit(t, s, `[
	  {"id":1,"intent":"accept"},{"id":2,"intent":"accept"},
	  {"id":3,"intent":"accept"},{"id":5,"intent":"accept"},
	  {"id":4,"intent":"dismiss","note":"that file is read-only at runtime"}
	]`)

	var accepts, dismisses int
	for _, e := range entries {
		switch e.Intent {
		case IntentAccept:
			accepts++
		case IntentDismiss:
			dismisses++
			if e.ID != 4 || e.Note != "that file is read-only at runtime" {
				t.Errorf("the exception should be member 4 with its note, got %+v", e)
			}
		}
	}
	if accepts != 4 || dismisses != 1 {
		t.Errorf("want 4 accepts and 1 dismiss, got %d and %d", accepts, dismisses)
	}
	// The artifact records the per-member calls, never the batch itself.
	for _, e := range entries {
		if e.ID == 0 {
			t.Error("a batch-level entry leaked into the artifact")
		}
	}
}

func TestBoard_TCBRD012_submit_answers_the_page(t *testing.T) {
	s := serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(1),
		Comments: []Comment{fullComment(1, "")},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/b/"+s.prefix+"/submit",
		strings.NewReader(`[{"id":1,"intent":"accept"}]`))
	s.Handler().ServeHTTP(rec, req)

	var got map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("submit response is not JSON: %v", err)
	}
	if !got["ok"] {
		t.Errorf("want an ok response so the page can confirm, got %s", rec.Body.String())
	}
}

// TestBoard_TCBRD009_serve_binds_announces_and_returns_on_submit drives
// Serve itself rather than Handler: the real net.Listen, the announced
// URL, the browser launch, a genuine HTTP round trip, and the shutdown
// that follows a submit. Everything else in this file bypasses Serve
// via httptest, so without this the wiring that makes the command work
// end to end has no coverage.
//
// NoBrowserForTesting is mandatory here, not incidental. A test run must
// never open a real window.
func TestBoard_TCBRD009_serve_binds_announces_and_returns_on_submit(t *testing.T) {
	NoBrowserForTesting(t)

	s := serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(142),
		Comments: []Comment{fullComment(1, ""), fullComment(2, "")},
	})

	urls := make(chan string, 1)
	done := make(chan []IntentEntry, 1)
	errs := make(chan error, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		entries, err := s.Serve(ctx, func(u string) { urls <- u })
		if err != nil {
			errs <- err
			return
		}
		done <- entries
	}()

	var url string
	select {
	case url = <-urls:
	case err := <-errs:
		t.Fatalf("Serve returned before announcing: %v", err)
	case <-ctx.Done():
		t.Fatal("Serve never announced a URL")
	}

	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Errorf("the board must bind loopback, got %q", url)
	}
	if !strings.Contains(url, "/b/"+s.prefix+"/") {
		t.Errorf("the announced URL must carry the path prefix, got %q", url)
	}

	// A real request over the bound socket, not httptest.
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
		t.Fatalf("Serve returned an error after submit: %v", err)
	case <-ctx.Done():
		t.Fatal("Serve did not return after the board was submitted")
	}
}

// TestBoard_TCBRD009_serve_returns_when_the_context_is_cancelled covers
// the other way out of Serve's blocking select: a caller that gives up
// before the developer submits.
func TestBoard_TCBRD009_serve_returns_when_the_context_is_cancelled(t *testing.T) {
	NoBrowserForTesting(t)

	s := serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(1),
		Comments: []Comment{fullComment(1, "")},
	})

	ctx, cancel := context.WithCancel(context.Background())
	announced := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		_, err := s.Serve(ctx, func(string) { close(announced) })
		result <- err
	}()

	<-announced
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("want context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after its context was cancelled")
	}
}

// TestBoard_TCBRD010_the_landing_path_redirects_without_carrying_the_secret
// pins the defence against leaking the board's path prefix through
// process arguments. The browser is launched with the landing URL, which
// any local user may read from argv; it must carry no secret, and it
// must still reach the board.
func TestBoard_TCBRD010_the_landing_path_redirects_without_carrying_the_secret(t *testing.T) {
	s := serverFor(t, Briefing{
		Repo: "acme/app", Scope: prScope(1),
		Comments: []Comment{fullComment(1, "")},
	})

	landing := s.LandingURL("127.0.0.1:1234")
	if strings.Contains(landing, s.prefix) {
		t.Fatalf("the URL handed to the browser must not carry the path prefix: %s", landing)
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, landingPath, nil))

	if rec.Code != http.StatusFound {
		t.Errorf("want 302 from the landing path, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/b/"+s.prefix+"/" {
		t.Errorf("the redirect must point at the board, got %q", got)
	}
	if strings.Contains(rec.Body.String(), "execSync") {
		t.Error("the redirect body leaked briefing content")
	}
}
