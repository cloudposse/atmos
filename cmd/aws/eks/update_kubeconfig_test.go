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
		name      string
		flagName  string
		shorthand string
	}{
		{name: "stack flag", flagName: "stack", shorthand: "s"},
		{name: "profile flag", flagName: "profile", shorthand: ""},
		{name: "name flag", flagName: "name", shorthand: ""},
		{name: "region flag", flagName: "region", shorthand: ""},
		{name: "kubeconfig flag", flagName: "kubeconfig", shorthand: ""},
		{name: "role-arn flag", flagName: "role-arn", shorthand: ""},
		{name: "dry-run flag", flagName: "dry-run", shorthand: ""},
		{name: "verbose flag", flagName: "verbose", shorthand: ""},
		{name: "alias flag", flagName: "alias", shorthand: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := flags.Lookup(tt.flagName)
			require.NotNil(t, flag, "flag %s should exist", tt.flagName)
			if tt.shorthand != "" {
				assert.Equal(t, tt.shorthand, flag.Shorthand)
			}
		})
	}
}

func TestUpdateKubeconfigCmd_UnexpectedFlags(t *testing.T) {
	// Verify that arbitrary flags do not exist.
	flags := updateKubeconfigCmd.Flags()

	unexpectedFlags := []string{
		"nonexistent-flag",
		"aws-profile",  // We use "profile" not "aws-profile".
		"cluster-name", // We use "name" not "cluster-name".
	}

	for _, flagName := range unexpectedFlags {
		t.Run(flagName, func(t *testing.T) {
			flag := flags.Lookup(flagName)
			assert.Nil(t, flag, "flag %s should not exist", flagName)
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
