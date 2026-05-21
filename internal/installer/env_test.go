package installer_test

import (
	"testing"

	"github.com/dmastrorillo/tai/internal/installer"
)

func TestIsTruthyEnv(t *testing.T) {
	const name = "TAI_TEST_TRUTHY"

	truthy := []string{"1", "true", "TRUE", "True", "yes", "YES", "on", "ON", "y", "Y", "t", "T", " 1 ", "  true  "}
	for _, v := range truthy {
		t.Setenv(name, v)
		if !installer.IsTruthyEnv(name) {
			t.Errorf("IsTruthyEnv(%q) = false, want true", v)
		}
	}

	falsy := []string{"", "0", "false", "FALSE", "no", "off", "n", "f", "maybe", "1.0", "yes please"}
	for _, v := range falsy {
		t.Setenv(name, v)
		if installer.IsTruthyEnv(name) {
			t.Errorf("IsTruthyEnv(%q) = true, want false", v)
		}
	}
}
