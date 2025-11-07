package exec

import (
	"os"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestMain(m *testing.M) {
	// Disable git root config search in tests to avoid finding repo config instead of fixture configs.
	// TestMain doesn't have *testing.T, manual cleanup via os.Unsetenv if needed.
	//nolint:lintroller // TestMain doesn't have *testing.T
	os.Setenv("ATMOS_GIT_ROOT_ENABLED", "false")
	code := m.Run()
	errUtils.Exit(code)
}
