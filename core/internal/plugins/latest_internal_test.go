package plugins

// Whitebox tests for unexported helpers in latest.go. Sibling
// fetch_test.go uses `package plugins_test` for blackbox coverage of
// the exported surface; this file covers the internal parser and
// comparator that the algorithm sits on top of.

import "testing"

// TestParseSemverNumeric pins the parser's contract across every
// edge case the algorithm depends on. Not tied to a TC-ID — this is
// a direct unit test for an internal helper; the user-facing TC-IDs
// (TC-REL-002..005) cover the integrated algorithm.
func TestParseSemverNumeric(t *testing.T) {
	cases := []struct {
		in        string
		wantOK    bool
		wantMajor int
		wantMinor int
		wantPatch int
	}{
		// Happy paths.
		{"v0.0.0", true, 0, 0, 0},
		{"v0.5.0", true, 0, 5, 0},
		{"v1.2.3", true, 1, 2, 3},
		{"v10.200.3000", true, 10, 200, 3000},

		// Missing `v` prefix.
		{"0.5.0", false, 0, 0, 0},
		{"", false, 0, 0, 0},

		// Wrong part count.
		{"v1.2", false, 0, 0, 0},
		{"v1.2.3.4", false, 0, 0, 0},
		{"v1", false, 0, 0, 0},

		// Pre-release / build-metadata suffixes — rejected as
		// defence in depth even though the JSON `prerelease` flag is
		// the authoritative filter.
		{"v1.2.3-rc.1", false, 0, 0, 0},
		{"v1.2.3+meta", false, 0, 0, 0},

		// Non-numeric parts.
		{"vabc.def.ghi", false, 0, 0, 0},
		{"v1.b.3", false, 0, 0, 0},

		// Leading zeros — strconv.Atoi accepts them, so the parser
		// does too. Documented behaviour, not a strict-semver
		// guarantee.
		{"v01.02.03", true, 1, 2, 3},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			maj, min, patch, ok := parseSemverNumeric(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parseSemverNumeric(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if maj != tc.wantMajor || min != tc.wantMinor || patch != tc.wantPatch {
				t.Errorf("parseSemverNumeric(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tc.in, maj, min, patch, tc.wantMajor, tc.wantMinor, tc.wantPatch)
			}
		})
	}
}

// TestCompareSemver pins the ordering used to pick the max-semver
// match. Not tied to a TC-ID.
func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b []int
		want int
	}{
		{[]int{1, 2, 3}, []int{1, 2, 3}, 0},
		{[]int{1, 2, 3}, []int{1, 2, 4}, -1},
		{[]int{1, 2, 4}, []int{1, 2, 3}, 1},
		{[]int{1, 3, 0}, []int{1, 2, 9}, 1},
		{[]int{2, 0, 0}, []int{1, 9, 9}, 1},
		{[]int{0, 0, 0}, []int{0, 0, 0}, 0},
		// The bestMajor=-1 sentinel used as the initial value in
		// LatestPrefixedTag: anything valid beats it.
		{[]int{0, 0, 0}, []int{-1, -1, -1}, 1},
	}
	for _, tc := range cases {
		got := compareSemver(tc.a[0], tc.a[1], tc.a[2], tc.b[0], tc.b[1], tc.b[2])
		if got != tc.want {
			t.Errorf("compareSemver(%v, %v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
