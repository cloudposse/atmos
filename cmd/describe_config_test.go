package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDescribeConfigCmd_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := describeConfigCmd.RunE(describeConfigCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "describe config command should return an error when called with invalid flags")
}
