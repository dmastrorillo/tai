package installer_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/internal/installer"
	"github.com/dmastrorillo/tai/internal/installer/installtest"
)

// runProbeSrc is the canonical bundled-command markdown reused by
// tests in this file, re-exported from installtest so assertions can
// compare against the same constant the fake bundle ships.
const runProbeSrc = installtest.ProbeSrc

// TestPrompt_default_skip: empty input (just newline) yields PromptSkip.
// The version string is the literal sentinel "vTEST" — the test asserts
// it appears in the prompt verbatim, which doubles as the regression
// guard for the parameterised version-threading.
func TestPrompt_default_skip(t *testing.T) {
	var stdout bytes.Buffer
	opts := installer.Options{
		Stdin:  strings.NewReader("\n"),
		Stdout: &stdout,
	}
	got, err := installer.Prompt(opts, "/tmp/probe.md", "vTEST")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != installer.PromptSkip {
		t.Fatalf("Prompt with empty input = %v, want PromptSkip", got)
	}
	if !strings.Contains(stdout.String(), "/tmp/probe.md") {
		t.Errorf("prompt text should name the path, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "vTEST") {
		t.Errorf("prompt text should embed the version, got %q", stdout.String())
	}
}

// TestPrompt_yes_overwrites: `y` (or `Y`) yields PromptOverwrite.
func TestPrompt_yes_overwrites(t *testing.T) {
	for _, ans := range []string{"y\n", "Y\n"} {
		var stdout bytes.Buffer
		opts := installer.Options{
			Stdin:  strings.NewReader(ans),
			Stdout: &stdout,
		}
		got, err := installer.Prompt(opts, "/tmp/probe.md", "vTEST")
		if err != nil {
			t.Fatalf("Prompt(%q): %v", ans, err)
		}
		if got != installer.PromptOverwrite {
			t.Errorf("Prompt(%q) = %v, want PromptOverwrite", ans, got)
		}
	}
}

// TestPrompt_no_skips: `n` (or other input) yields PromptSkip.
func TestPrompt_no_skips(t *testing.T) {
	for _, ans := range []string{"n\n", "N\n", "no\n", "garbage\n"} {
		var stdout bytes.Buffer
		opts := installer.Options{
			Stdin:  strings.NewReader(ans),
			Stdout: &stdout,
		}
		got, err := installer.Prompt(opts, "/tmp/probe.md", "vTEST")
		if err != nil {
			t.Fatalf("Prompt(%q): %v", ans, err)
		}
		if got != installer.PromptSkip {
			t.Errorf("Prompt(%q) = %v, want PromptSkip", ans, got)
		}
	}
}

// TestFormatSummary_install_with_outcomes: summary correctly lists
// installed, updated, skipped, and prompted-skipped buckets.
func TestFormatSummary_install_with_outcomes(t *testing.T) {
	results := []installer.Result{
		{Verb: "alpha", Outcome: installer.OutcomeInstalled},
		{Verb: "beta", Outcome: installer.OutcomeUpdated},
		{Verb: "gamma", Outcome: installer.OutcomeSkippedUpToDate},
		{Verb: "delta", Outcome: installer.OutcomePromptedSkipped},
	}
	got := installer.FormatSummary(results, true)
	for _, want := range []string{
		"Installed: 1 command (alpha)",
		"Updated: 1 command (beta)",
		"Skipped: 1 command (up to date)",
		"Prompted-skipped: 1 command (delta)",
		"[exit 0]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q\nfull:\n%s", want, got)
		}
	}
}

// TestFormatSummary_uninstall_with_outcomes: summary correctly lists
// removed, prompted-skipped, and not-found buckets.
func TestFormatSummary_uninstall_with_outcomes(t *testing.T) {
	results := []installer.Result{
		{Verb: "alpha", Outcome: installer.OutcomeRemoved},
		{Verb: "beta", Outcome: installer.OutcomePromptedSkipped},
		{Verb: "gamma", Outcome: installer.OutcomeNotFound},
	}
	got := installer.FormatSummary(results, false)
	for _, want := range []string{
		"Removed: 1 command (alpha)",
		"Prompted-skipped: 1 command (beta)",
		"Not-found: 1 command (gamma)",
		"[exit 0]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q\nfull:\n%s", want, got)
		}
	}
}

// TestFormatSummary_no_bundled_commands: when there are no results
// (e.g. no bundled commands in this build), the summary falls back to
// a single "No bundled commands" line plus the exit tag.
func TestFormatSummary_no_bundled_commands(t *testing.T) {
	got := installer.FormatSummary(nil, true)
	if !strings.Contains(got, "No bundled commands in this build.") {
		t.Errorf("expected 'No bundled commands in this build.' fallback, got:\n%s", got)
	}
	if !strings.Contains(got, "[exit 0]") {
		t.Errorf("expected [exit 0] footer, got:\n%s", got)
	}
}

// TestInstall_interactive_prompt_accept: with IsTTY=true and a "y"
// answer on stdin, Install overwrites a user-modified file.
func TestInstall_interactive_prompt_accept(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "probe.md")
	if err := os.WriteFile(target, []byte("user content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	opts := installer.Options{
		TargetDir: dir,
		IsTTY:     true,
		Stdin:     strings.NewReader("y\n"),
		Stdout:    &stdout,
		Bundle:    installtest.NewSingleVerb(),
	}
	results, err := installer.Install(opts)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(results) != 1 || results[0].Outcome != installer.OutcomeUpdated {
		t.Fatalf("expected one Updated result, got %v", results)
	}

	if !strings.Contains(stdout.String(), "[y/N]") {
		t.Errorf("stdout missing prompt text: %q", stdout.String())
	}
	got, _ := os.ReadFile(target)
	if string(got) != runProbeSrc {
		t.Fatalf("file not overwritten\nwant: %q\ngot:  %q", runProbeSrc, got)
	}
}

// TestInstall_interactive_prompt_decline: with IsTTY=true and an empty
// answer on stdin (just newline), Install leaves the user-modified
// file alone and reports Prompted-skipped.
func TestInstall_interactive_prompt_decline(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "probe.md")
	if err := os.WriteFile(target, []byte("user content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	opts := installer.Options{
		TargetDir: dir,
		IsTTY:     true,
		Stdin:     strings.NewReader("\n"),
		Stdout:    &stdout,
		Bundle:    installtest.NewSingleVerb(),
	}
	results, err := installer.Install(opts)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(results) != 1 || results[0].Outcome != installer.OutcomePromptedSkipped {
		t.Fatalf("expected one Prompted-skipped result, got %v", results)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "user content\n" {
		t.Fatalf("file was modified despite decline, got %q", got)
	}
}

// TestFormatSummary_collapses_long_verb_lists: when N>5, the verb list
// becomes "(…)" instead of being fully expanded.
func TestFormatSummary_collapses_long_verb_lists(t *testing.T) {
	var results []installer.Result
	for _, v := range []string{"a", "b", "c", "d", "e", "f"} {
		results = append(results, installer.Result{Verb: v, Outcome: installer.OutcomeInstalled})
	}
	got := installer.FormatSummary(results, true)
	if !strings.Contains(got, "Installed: 6 commands (…)") {
		t.Errorf("expected collapsed verb list, got:\n%s", got)
	}
}
