package aks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateKubeconfigCmd_Error(t *testing.T) {
	err := updateKubeconfigCmd.RunE(updateKubeconfigCmd, []string{})
	assert.Error(t, err, "azure aks update-kubeconfig command should return an error when called with no parameters")
}

func TestUpdateKubeconfigCmd_Flags(t *testing.T) {
	flags := updateKubeconfigCmd.Flags()

	tests := []struct {
		name      string
		flagName  string
		shorthand string
	}{
		{name: "cluster-name flag", flagName: "cluster-name", shorthand: ""},
		{name: "resource-group flag", flagName: "resource-group", shorthand: ""},
		{name: "subscription-id flag", flagName: "subscription-id", shorthand: ""},
		{name: "kubeconfig flag", flagName: "kubeconfig", shorthand: ""},
		{name: "alias flag", flagName: "alias", shorthand: ""},
		{name: "integration flag", flagName: "integration", shorthand: ""},
		{name: "identity flag", flagName: "identity", shorthand: "i"},
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
	flags := updateKubeconfigCmd.Flags()

	unexpectedFlags := []string{
		"nonexistent-flag",
		"name",   // We use "cluster-name" not "name" for AKS (unlike EKS).
		"region", // AKS uses "resource-group", not "region".
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

func TestUpdateKubeconfigCmd_ParentIsAksCmd(t *testing.T) {
	assert.NotNil(t, updateKubeconfigCmd.Parent())
	if updateKubeconfigCmd.Parent() != nil {
		assert.Equal(t, "aks", updateKubeconfigCmd.Parent().Name())
	}
}
