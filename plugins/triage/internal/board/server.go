package board

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

//go:embed page.html
var pageFS embed.FS

var pageTemplate = template.Must(template.ParseFS(pageFS, "page.html"))

// listen is the seam the bind-failure case is reached through. A
// kernel-assigned port has no deterministic way to fail, and the
// TRIAGE_BOARD_UNAVAILABLE exit code it pins is in an append-only
// registry, so the case needs a test and the test needs this.
var listen = net.Listen

// ListenFailureForTesting makes the next bind fail with err for the
// lifetime of t. The testing.TB parameter is the guard: a production
// binary that imports `testing` deliberately is a code-review red flag.
func ListenFailureForTesting(t testing.TB, err error) {
	t.Helper()
	prev := listen
	listen = func(string, string) (net.Listener, error) { return nil, err }
	t.Cleanup(func() { listen = prev })
}

// openBrowser is overridable so a test can assert the server survives a
// launch failure without depending on the host having no browser, and so
// no test run ever pops a real window.
var openBrowser = launchBrowser

// NoBrowserForTesting stops the board opening a window for the lifetime
// of t. The testing.TB parameter is the guard: a production binary that
// imports `testing` deliberately is a code-review red flag.
func NoBrowserForTesting(t testing.TB) {
	t.Helper()
	prev := openBrowser
	openBrowser = func(string) {}
	t.Cleanup(func() { openBrowser = prev })
}

// Server serves one board for one briefing and blocks until submit.
type Server struct {
	briefing Briefing
	groups   []Group
	prefix   string

	once      sync.Once
	submitted chan submission
}

type submission struct {
	entries []IntentEntry
}

// NewServer prepares a board for the briefing. Ordering happens here so
// the page and the artifact agree on the queue.
func NewServer(b Briefing) (*Server, error) {
	prefix, err := randomPrefix()
	if err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err,
			"generating the board's path prefix")
	}
	return &Server{
		briefing:  b,
		groups:    Order(b),
		prefix:    prefix,
		submitted: make(chan submission, 1),
	}, nil
}

// randomPrefix returns 32 hex characters from a cryptographically secure
// source. Loopback is not a boundary on a shared machine — any local
// process can reach 127.0.0.1 on any port — so the path is what keeps
// the briefing private.
func randomPrefix() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Handler is the board's routing surface, exported so handler tests can
// drive it without binding a port.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	base := "/b/" + s.prefix
	mux.HandleFunc(base+"/", s.handlePage)
	mux.HandleFunc(base+"/submit", s.handleSubmit)
	root := http.NewServeMux()
	root.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The landing path carries no secret, so it is safe to hand to
		// a browser on the command line. It redirects over the socket
		// that already holds the prefix.
		if r.URL.Path == landingPath {
			http.Redirect(w, r, base+"/", http.StatusFound)
			return
		}
		if !strings.HasPrefix(r.URL.Path, base+"/") {
			http.NotFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	return root
}

// landingPath is the secret-free entry point. Requests to it are
// redirected to the prefixed board over the already-bound socket.
const landingPath = "/open"

// URL is the board's entry point, valid once Serve has bound. It
// carries the path prefix and is what the developer is shown.
func (s *Server) URL(addr string) string {
	return fmt.Sprintf("http://%s/b/%s/", addr, s.prefix)
}

// LandingURL is what the browser is launched with. It deliberately
// omits the prefix: a browser is started by exec'ing `open`/`xdg-open`
// with the URL as an argument, and process arguments are readable by
// every other local user on macOS and most Linux configurations. Handing
// the secret to argv would publish the very thing the prefix protects.
func (s *Server) LandingURL(addr string) string {
	return "http://" + addr + landingPath
}

type pageData struct {
	Repo       string
	ScopeLabel string
	Groups     []Group
	Total      int
	SubmitPath string
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/submit") {
		http.NotFound(w, r)
		return
	}
	data := pageData{
		Repo:       s.briefing.Repo,
		ScopeLabel: scopeLabel(s.briefing.Scope),
		Groups:     s.groups,
		Total:      len(s.briefing.Comments),
		SubmitPath: "/b/" + s.prefix + "/submit",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.Execute(w, data); err != nil {
		// The one way the board fails after binding successfully, and
		// without this the developer sees a blank page and no trail.
		slog.Error("rendering the board page failed",
			"path", r.URL.Path, "error", err)
		http.Error(w, "rendering the board failed", http.StatusInternalServerError)
	}
}

// postedIntent is one decision as the page posts it. A batch-level call
// is expanded in the page, so what arrives here is already per-member —
// the artifact records the calls that produced a split, not the split.
type postedIntent struct {
	ID     int    `json:"id"`
	Intent string `json:"intent"`
	Note   string `json:"note"`
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var posted []postedIntent
	if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
		http.Error(w, "malformed submission", http.StatusBadRequest)
		return
	}

	byID := map[int]postedIntent{}
	for _, p := range posted {
		byID[p.ID] = p
	}

	// Every briefed comment gets an entry. A comment the developer never
	// touched is `unanswered`, which is what sends it to the loop.
	entries := make([]IntentEntry, 0, len(s.briefing.Comments))
	for _, g := range s.groups {
		for _, c := range g.Comments {
			e := IntentEntry{ID: c.ID, Intent: IntentUnanswered}
			if p, ok := byID[c.ID]; ok {
				if p.Intent == IntentAccept || p.Intent == IntentDismiss {
					e.Intent = p.Intent
				}
				e.Note = strings.TrimSpace(p.Note)
			}
			entries = append(entries, e)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))

	s.once.Do(func() { s.submitted <- submission{entries: entries} })
}

// Serve binds loopback on a kernel-assigned port, announces the URL
// through announce, opens a browser best-effort, and blocks until the
// developer submits or ctx is cancelled. It returns the entries the
// caller writes as the artifact.
func (s *Server) Serve(ctx context.Context, announce func(url string)) ([]IntentEntry, error) {
	ln, err := listen("tcp", "127.0.0.1:0")
	if err != nil {
		// The cause is repeated into the message because the error
		// template renders Msg and nothing else, and in a detached
		// board this message is all the launching command has to pass
		// on — this process's stderr is /dev/null.
		return nil, errcode.Wrapf(errcode.TriageBoardUnavailable, err,
			"binding a loopback listener for the board: %s", err).
			WithHelp(
				"check whether a local firewall or sandbox blocks binding 127.0.0.1",
				"the board needs no outbound network access, only a loopback socket",
			)
	}
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()
	announce(s.URL(addr))
	openBrowser(s.LandingURL(addr))

	srv := &http.Server{Handler: s.Handler()}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	select {
	case sub := <-s.submitted:
		return sub.entries, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// launchBrowser is best-effort. A headless box has no browser and that
// is not an error: the URL is already on stdout and can be forwarded.
func launchBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func scopeLabel(s Scope) string {
	if s.Kind == "branch" {
		return "branch " + s.Branch
	}
	if s.PR != nil {
		return fmt.Sprintf("PR #%d", *s.PR)
	}
	return "PR"
}
