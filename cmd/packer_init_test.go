package cmd

import (
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"
)

func TestPackerInitCmd(t *testing.T) {
	_ = NewTestKit(t)

	skipIfPackerNotInstalled(t)

	workDir := "../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.WarnLevel)

	RootCmd.SetArgs([]string{"packer", "init", "aws/bastion", "-s", "nonprod"})
	err := Execute()
	assert.NoError(t, err, "'TestPackerInitCmd' should execute without error")
}
