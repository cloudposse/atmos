package helmfile

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// skipIfHelmfileNotInstalled skips the test if helmfile is not installed.
func skipIfHelmfileNotInstalled(t *testing.T) {
	t.Helper()
	_, err := exec.LookPath("helmfile")
	if err != nil {
		t.Skip("helmfile not installed:", err)
	}
}

func TestHelmfileCommands_Error(t *testing.T) {
	skipIfHelmfileNotInstalled(t)
	stacksPath := "../../tests/fixtures/scenarios/stack-templates"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := helmfileApplyCmd.RunE(helmfileApplyCmd, []string{})
	assert.Error(t, err, "helmfile apply command should return an error when called with no parameters")

	err = helmfileDestroyCmd.RunE(helmfileDestroyCmd, []string{})
	assert.Error(t, err, "helmfile destroy command should return an error when called with no parameters")

	err = helmfileDiffCmd.RunE(helmfileDiffCmd, []string{})
	assert.Error(t, err, "helmfile diff command should return an error when called with no parameters")

	err = helmfileSyncCmd.RunE(helmfileSyncCmd, []string{})
	assert.Error(t, err, "helmfile sync command should return an error when called with no parameters")
}
