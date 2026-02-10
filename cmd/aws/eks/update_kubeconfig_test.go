package eks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateKubeconfigCmd_Error(t *testing.T) {
	err := updateKubeconfigCmd.RunE(updateKubeconfigCmd, []string{})
	assert.Error(t, err, "aws eks update-kubeconfig command should return an error when called with no parameters")
}

func TestUpdateKubeconfigCmd_Flags(t *testing.T) {
	// Verify all expected flags are registered.
	flags := updateKubeconfigCmd.Flags()

	tests := []struct {
		name       string
		flagName   string
		shorthand  string
		expectFlag bool
	}{
		{name: "stack flag", flagName: "stack", shorthand: "s", expectFlag: true},
		{name: "profile flag", flagName: "profile", shorthand: "", expectFlag: true},
		{name: "name flag", flagName: "name", shorthand: "", expectFlag: true},
		{name: "region flag", flagName: "region", shorthand: "", expectFlag: true},
		{name: "kubeconfig flag", flagName: "kubeconfig", shorthand: "", expectFlag: true},
		{name: "role-arn flag", flagName: "role-arn", shorthand: "", expectFlag: true},
		{name: "dry-run flag", flagName: "dry-run", shorthand: "", expectFlag: true},
		{name: "verbose flag", flagName: "verbose", shorthand: "", expectFlag: true},
		{name: "alias flag", flagName: "alias", shorthand: "", expectFlag: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := flags.Lookup(tt.flagName)
			if tt.expectFlag {
				require.NotNil(t, flag, "flag %s should exist", tt.flagName)
				if tt.shorthand != "" {
					assert.Equal(t, tt.shorthand, flag.Shorthand)
				}
			}
		})
	}
}

func TestUpdateKubeconfigCmd_CommandMetadata(t *testing.T) {
	assert.Equal(t, "update-kubeconfig", updateKubeconfigCmd.Use)
	assert.Contains(t, updateKubeconfigCmd.Short, "Update")
	assert.Contains(t, updateKubeconfigCmd.Short, "kubeconfig")
	assert.NotEmpty(t, updateKubeconfigCmd.Long)
}

func TestUpdateKubeconfigCmd_FParseErrWhitelist(t *testing.T) {
	// This command should NOT whitelist unknown flags (strict parsing).
	assert.False(t, updateKubeconfigCmd.FParseErrWhitelist.UnknownFlags)
}

func TestUpdateKubeconfigParser(t *testing.T) {
	// Verify the parser is initialized.
	require.NotNil(t, updateKubeconfigParser, "updateKubeconfigParser should be initialized")
}
