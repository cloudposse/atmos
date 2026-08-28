package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGCPCommandProvider verifies the GCP command provider contract.
func TestGCPCommandProvider(t *testing.T) {
	provider := &GCPCommandProvider{}
	assert.Equal(t, "gcp", provider.GetName())
	assert.Equal(t, "Cloud Integration", provider.GetGroup())
	assert.False(t, provider.IsExperimental())
	assert.Nil(t, provider.GetAliases())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.Same(t, gcpCmd, provider.GetCommand())
}

// TestGCPCommandHierarchyAndHelp verifies GCP subcommand discovery and help text.
func TestGCPCommandHierarchyAndHelp(t *testing.T) {
	assert.Equal(t, "gcp", gcpCmd.Use)
	assert.Contains(t, gcpCmd.Short, "GCP-specific")
	assert.Contains(t, gcpCmd.Example, "atmos gcp gke token")
	gkeCmd, _, err := gcpCmd.Find([]string{"gke"})
	require.NoError(t, err)
	assert.Equal(t, "gke", gkeCmd.Name())
	token, _, err := gcpCmd.Find([]string{"gke", "token"})
	require.NoError(t, err)
	assert.Equal(t, "token", token.Name())
	assert.Contains(t, token.Long, "ExecCredential")
}
