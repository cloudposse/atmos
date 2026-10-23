package terraform

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/terraform/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerraformStackLifecycleRegistration(t *testing.T) {
	deploy, ok := lifecycle.DefaultRegistry.Lookup("terraform", lifecycle.Deploy)
	require.True(t, ok)
	assert.Equal(t, []string{"terraform", "deploy"}, deploy.Path)
	assert.Equal(t, []string{"before.terraform.deploy", "after.terraform.deploy"}, deploy.HookEvents)
	require.NotNil(t, deploy.NewResourceGuard)
	assert.Contains(t, deploy.WorkerEnvironment(2), plugin.DisableCacheEnv+"=true")
	assert.Contains(t, deploy.WorkerEnvironment(1), plugin.DisableCacheEnv+"=false")
	destroy, ok := lifecycle.DefaultRegistry.Lookup("terraform", lifecycle.Destroy)
	require.True(t, ok)
	assert.Equal(t, []string{"-auto-approve"}, destroy.ApprovalArgs)
	assert.Empty(t, destroy.HookEvents)
	_, ok = lifecycle.DefaultRegistry.Lookup("terraform", lifecycle.Build)
	assert.False(t, ok, "unregistered events must not be inferred")
}
