package doctor

import (
	"os"
	"testing"
)

// TestMain isolates the user-level Atmos configuration. Discovery reads the home and system
// configuration, so a developer's real ~/.atmos must not change what fixtures report.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "doctor-home-") //nolint:lintroller // TestMain has no *testing.T.
	if err != nil {
		panic(err)
	}
	for _, name := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME"} {
		_ = os.Setenv(name, home) //nolint:lintroller // TestMain has no *testing.T.
	}
	for _, name := range []string{"ATMOS_CLI_CONFIG_PATH", "ATMOS_BASE_PATH", "ATMOS_CONFIG", "ATMOS_CONFIG_PATH"} {
		_ = os.Unsetenv(name)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
