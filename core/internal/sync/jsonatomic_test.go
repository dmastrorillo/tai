package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteJSONAtomic_writes_indented_json_with_trailing_newline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "out.json")

	if err := writeJSONAtomic(path, map[string]string{"key": "value"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("want trailing newline")
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["key"] != "value" {
		t.Errorf("round-trip mismatch: %v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("want 0644 perms (parity with prior WriteFile), got %o", perm)
	}
}

// Concurrent savers of the same path must not interfere: the old
// fixed "<path>.tmp" staging name let writer B's pre-clean delete
// writer A's in-flight tmp, failing A's rename with ENOENT even
// though nothing was actually corrupted. Unique staging names make
// every writer succeed and leave a valid final file. This matters in
// production because the background poll runs on every tai
// invocation, so back-to-back or scripted-parallel tai commands
// routinely overlap SaveState calls.
func TestWriteJSONAtomic_concurrent_writers_all_succeed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")

	const writers = 20
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = writeJSONAtomic(path, map[string]int{"writer": i})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("writer %d failed: %v", i, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]int
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("final file not valid JSON: %v\n%s", err, data)
	}

	// No staging litter left behind on the success path.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "out.json" {
			t.Errorf("staging litter left behind: %s", e.Name())
		}
	}
}
