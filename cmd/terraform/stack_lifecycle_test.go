package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/terraform/plugin"
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

func TestTerraformStackTestRegistration(t *testing.T) {
	handler, ok := lifecycle.DefaultRegistry.Lookup("terraform", lifecycle.Test)
	require.True(t, ok)
	args, err := handler.Arguments(&dependency.Node{Type: "terraform", Stack: "dev", Component: "network"})
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform", "test", "--stack=dev", "network"}, args)
	assert.Equal(t, []string{"before.terraform.test", "after.terraform.test"}, handler.HookEvents)
	assert.Empty(t, handler.ApprovalArgs, "terraform test has no deployment approval step")
	require.NotNil(t, handler.NewResourceGuard)
	assert.NotNil(t, handler.NewResourceGuard(&schema.AtmosConfiguration{}))
	assert.Equal(t, []string{plugin.DisableCacheEnv + "=true"}, handler.WorkerEnvironment(2))
	assert.Equal(t, []string{plugin.DisableCacheEnv + "=false"}, handler.WorkerEnvironment(1))
}
