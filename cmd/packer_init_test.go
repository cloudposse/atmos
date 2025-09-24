package cmd

import (
	"os/exec"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestPackerInitCmd(t *testing.T) {
	// Skip test if packer binary is not available
	if _, err := exec.LookPath("packer"); err != nil {
		t.Skipf("Skipping test because packer binary is not found in PATH: %v", err)
	}

	workDir := "../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.WarnLevel)

	RootCmd.SetArgs([]string{"packer", "init", "aws/bastion", "-s", "nonprod"})
	err := Execute()
	assert.NoError(t, err, "'TestPackerInitCmd' should execute without error")
}
