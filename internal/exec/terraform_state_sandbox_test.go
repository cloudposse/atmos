package exec

import (
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

func setupTerraformStateSandbox(t *testing.T, workDir string) string {
	t.Helper()

	sandbox, err := testhelpers.SetupSandbox(t, workDir)
	if err != nil {
		t.Fatalf("failed to setup sandbox for %q: %v", workDir, err)
	}
	t.Cleanup(sandbox.Cleanup)

	for key, value := range sandbox.GetEnvironmentVariables() {
		t.Setenv(key, value)
	}

	return filepath.Join(sandbox.ComponentsPath, "terraform")
}
