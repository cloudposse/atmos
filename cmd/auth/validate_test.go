package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthValidateCommand_Structure(t *testing.T) {
	assert.Equal(t, "validate", authValidateCmd.Use)
	assert.NotEmpty(t, authValidateCmd.Short)
	assert.NotEmpty(t, authValidateCmd.Long)
	assert.NotNil(t, authValidateCmd.RunE)

	// Check verbose flag exists.
	verboseFlag := authValidateCmd.Flags().Lookup("verbose")
	assert.NotNil(t, verboseFlag)
	assert.Equal(t, "v", verboseFlag.Shorthand)
}

func TestValidateParser_Initialization(t *testing.T) {
	// validateParser should be initialized in init().
	assert.NotNil(t, validateParser)
}

func TestAuthValidateCommand_FParseErrWhitelist(t *testing.T) {
	// Verify FParseErrWhitelist is configured.
	assert.False(t, authValidateCmd.FParseErrWhitelist.UnknownFlags)
}

func TestAuthValidateCommand_VerboseFlagDefault(t *testing.T) {
	// Verbose flag should default to false.
	verboseFlag := authValidateCmd.Flags().Lookup("verbose")
	assert.NotNil(t, verboseFlag)
	assert.Equal(t, "false", verboseFlag.DefValue)
}

// TestExecuteAuthValidateCommand_SmokeNoConfig exercises the validate
// orchestrator from a directory without an atmos.yaml.
//
// Contract: the function must not panic. Whether an error surfaces depends on
// whether atmos pickup defaults find an upstream atmos.yaml — both outcomes
// are acceptable. The config-load error-wrap sentinel
// (ErrFailedToInitializeAtmosConfig) is exercised in
// TestLoadAuthManagerForEnv_SmokeFromEmptyTempDir.
func TestExecuteAuthValidateCommand_SmokeNoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := authValidateCmd
	cmd.SetContext(context.Background())

	assert.NotPanics(t, func() {
		_ = executeAuthValidateCommand(cmd, nil)
	})
}

// TestExecuteAuthValidateCommand_WithMockAuth exercises validate end-to-end
// against the mock auth fixture. The mock config is valid so the function
// must return nil (success).
func TestExecuteAuthValidateCommand_WithMockAuth(t *testing.T) {
	setupMockAuthFixture(t)

	cmd := authValidateCmd
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags(nil))

	err := executeAuthValidateCommand(cmd, nil)
	assert.NoError(t, err,
		"validate of a well-formed mock/aws auth config must succeed")
}
