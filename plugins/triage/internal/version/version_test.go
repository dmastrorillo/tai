package version

import (
	"runtime/debug"
	"testing"
)

// TestResolveVersion mirrors the core/internal/version test of the
// same name. Triage's helper is structurally identical; the test
// exists per-package to guarantee both packages keep the same
// precedence rules if either ever diverges.
func TestResolveVersion(t *testing.T) {
	cases := []struct {
		name   string
		linked string
		info   *debug.BuildInfo
		want   string
	}{
		{"linker_wins", "v0.6.0", &debug.BuildInfo{Main: debug.Module{Version: "v0.5.0"}}, "v0.6.0"},
		{"linker_wins_even_without_buildinfo", "v0.6.0", nil, "v0.6.0"},
		{"dev_no_buildinfo", "dev", nil, "dev"},
		{"dev_buildinfo_devel", "dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev"},
		{"dev_buildinfo_empty", "dev", &debug.BuildInfo{Main: debug.Module{Version: ""}}, "dev"},
		{"dev_buildinfo_real_tag", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "v0.1.0"},
		{"dev_buildinfo_prerelease", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0-rc.1"}}, "v0.2.0-rc.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveVersion(tc.linked, tc.info)
			if got != tc.want {
				t.Errorf("resolveVersion(%q, %+v) = %q, want %q",
					tc.linked, tc.info, got, tc.want)
			}
		})
	}
}
