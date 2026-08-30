package scope

import "testing"

// ParentColumn/ParentTable are the single source of the Kind → SQL
// name mapping. Before they existed the 2-line `col := "pr_id"; if
// branch { col = "branch_id" }` ladder was reimplemented at five call
// sites across the cmd and triage packages.
func TestScope_ParentColumn_and_ParentTable(t *testing.T) {
	cases := []struct {
		kind      Kind
		wantCol   string
		wantTable string
	}{
		{KindPR, "pr_id", "prs"},
		{KindBranch, "branch_id", "branches"},
	}
	for _, tc := range cases {
		s := Scope{Kind: tc.kind}
		if got := s.ParentColumn(); got != tc.wantCol {
			t.Errorf("Scope{%s}.ParentColumn() = %q, want %q", tc.kind, got, tc.wantCol)
		}
		if got := s.ParentTable(); got != tc.wantTable {
			t.Errorf("Scope{%s}.ParentTable() = %q, want %q", tc.kind, got, tc.wantTable)
		}
	}
}
