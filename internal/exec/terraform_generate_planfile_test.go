package exec

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecuteTerraformGeneratePlanfile(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/terraform-generate-planfile"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")
	// Unset env values after testing
	defer func() {
		os.Unsetenv("ATMOS_BASE_PATH")
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	}()

	err = ExecuteTerraformGeneratePlanfile(
		"component-1",
		"nonprod",
		"",
		true,
		true,
		nil,
		schema.ConfigAndStacksInfo{},
	)

	assert.NoError(t, err)
}
