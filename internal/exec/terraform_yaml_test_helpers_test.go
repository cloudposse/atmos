package exec

import (
	"testing"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

func setupTerraformYamlFunctionSandbox(t *testing.T, workDir string) {
	t.Helper()

	sandbox, err := testhelpers.SetupSandbox(t, workDir)
	if err != nil {
		t.Fatalf("Failed to setup sandbox: %v", err)
	}
	t.Cleanup(sandbox.Cleanup)

	for k, v := range sandbox.GetEnvironmentVariables() {
		t.Setenv(k, v)
	}
}
