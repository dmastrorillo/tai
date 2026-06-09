package version

import (
	"runtime/debug"
	"testing"
)

// TestResolveVersion pins the linker-vs-BuildInfo precedence used at
// package init. Not tied to a TC-ID — this is a direct unit test for
// the helper inside the version package's init() block.
//
// Rules:
//   - Linker injection wins. Any non-"dev" `linked` value passes
//     through verbatim.
//   - "dev" + no BuildInfo, or BuildInfo with empty/"(devel)"
//     Main.Version: stay at "dev" (local/test build).
//   - "dev" + real Main.Version: use the module version. This is the
//     `go install github.com/.../core/cmd/tai@vX.Y.Z` case where the
//     ldflags injection doesn't run but BuildInfo carries the tag.
func TestResolveVersion(t *testing.T) {
	cases := []struct {
		name   string
		linked string
		info   *debug.BuildInfo
		want   string
	}{
		{
			name:   "linker_wins",
			linked: "v0.6.0",
			info:   &debug.BuildInfo{Main: debug.Module{Version: "v0.5.0"}},
			want:   "v0.6.0",
		},
		{
			name:   "linker_wins_even_without_buildinfo",
			linked: "v0.6.0",
			info:   nil,
			want:   "v0.6.0",
		},
		{
			name:   "dev_no_buildinfo",
			linked: "dev",
			info:   nil,
			want:   "dev",
		},
		{
			name:   "dev_buildinfo_devel",
			linked: "dev",
			info:   &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			want:   "dev",
		},
		{
			name:   "dev_buildinfo_empty",
			linked: "dev",
			info:   &debug.BuildInfo{Main: debug.Module{Version: ""}},
			want:   "dev",
		},
		{
			name:   "dev_buildinfo_real_tag",
			linked: "dev",
			info:   &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}},
			want:   "v0.1.0",
		},
		{
			name:   "dev_buildinfo_prerelease",
			linked: "dev",
			info:   &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0-rc.1"}},
			want:   "v0.2.0-rc.1",
		},
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
