package describe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestProcessComponentInStack(t *testing.T) {
	component := "infra/vpc"
	stack := "tenant1-ue2-dev"

	result, err := ProcessComponentInStack(component, stack, "", "")
	assert.Nil(t, err)
	assert.NotNil(t, result)

	resultYaml, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if resultYaml != "" {
				t.Logf("Component section:\n%s", resultYaml)
			} else {
				t.Logf("Component section (raw): %+v", result)
			}
		}
	})
}

func TestProcessComponentInStackReturnsVars(t *testing.T) {
	component := "top-level-component1"
	stack := "tenant1-ue2-dev"

	result, err := ProcessComponentInStack(component, stack, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "result should contain 'vars' section")
	assert.Equal(t, "tenant1", vars["tenant"])
	assert.Equal(t, "ue2", vars["environment"])
	assert.Equal(t, "dev", vars["stage"])
}

func TestProcessComponentInStackReturnsWorkspace(t *testing.T) {
	component := "test/test-component-override-3"
	stack := "tenant1-ue2-dev"

	result, err := ProcessComponentInStack(component, stack, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)

	workspace, ok := result["workspace"].(string)
	require.True(t, ok, "result should contain 'workspace'")
	assert.NotEmpty(t, workspace)
}

func TestProcessComponentInStackInvalidComponent(t *testing.T) {
	component := "nonexistent-component"
	stack := "tenant1-ue2-dev"

	_, err := ProcessComponentInStack(component, stack, "", "")
	assert.NotNil(t, err)
}

func TestProcessComponentInStackInvalidStack(t *testing.T) {
	component := "infra/vpc"
	stack := "nonexistent-stack"

	_, err := ProcessComponentInStack(component, stack, "", "")
	assert.NotNil(t, err)
}

func TestProcessComponentFromContext(t *testing.T) {
	result, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:   "infra/vpc",
		Tenant:      "tenant1",
		Environment: "ue2",
		Stage:       "dev",
	})
	assert.Nil(t, err)
	assert.NotNil(t, result)

	resultYaml, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if resultYaml != "" {
				t.Logf("Component section:\n%s", resultYaml)
			} else {
				t.Logf("Component section (raw): %+v", result)
			}
		}
	})
}

func TestProcessComponentFromContextReturnsVars(t *testing.T) {
	result, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:   "top-level-component1",
		Tenant:      "tenant1",
		Environment: "ue2",
		Stage:       "dev",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "result should contain 'vars' section")
	assert.Equal(t, "tenant1", vars["tenant"])
	assert.Equal(t, "ue2", vars["environment"])
	assert.Equal(t, "dev", vars["stage"])
}

func TestProcessComponentFromContextInvalidComponent(t *testing.T) {
	_, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:   "nonexistent-component",
		Tenant:      "tenant1",
		Environment: "ue2",
		Stage:       "dev",
	})
	assert.NotNil(t, err)
}

func TestProcessComponentFromContextInvalidContext(t *testing.T) {
	_, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:   "infra/vpc",
		Tenant:      "nonexistent",
		Environment: "xxx",
		Stage:       "yyy",
	})
	assert.NotNil(t, err)
}

func TestProcessComponentInStackMatchesFromContext(t *testing.T) {
	component := "infra/vpc"
	stack := "tenant1-ue2-dev"

	resultByStack, err := ProcessComponentInStack(component, stack, "", "")
	require.NoError(t, err)

	resultByContext, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:   component,
		Tenant:      "tenant1",
		Environment: "ue2",
		Stage:       "dev",
	})
	require.NoError(t, err)

	// Both methods should return the same component configuration
	resultByStackYaml, err := u.ConvertToYAML(resultByStack)
	require.NoError(t, err)
	resultByContextYaml, err := u.ConvertToYAML(resultByContext)
	require.NoError(t, err)

	assert.Equal(t, resultByStackYaml, resultByContextYaml,
		"ProcessComponentInStack and ProcessComponentFromContext should return the same result")
}

func TestProcessComponentInStackTenant2(t *testing.T) {
	component := "infra/vpc"
	stack := "tenant2-ue2-dev"

	result, err := ProcessComponentInStack(component, stack, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "result should contain 'vars' section")
	assert.Equal(t, "tenant2", vars["tenant"])
	assert.Equal(t, "ue2", vars["environment"])
	assert.Equal(t, "dev", vars["stage"])
}

func TestProcessComponentFromContextTenant2(t *testing.T) {
	result, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:   "infra/vpc",
		Tenant:      "tenant2",
		Environment: "ue2",
		Stage:       "dev",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "result should contain 'vars' section")
	assert.Equal(t, "tenant2", vars["tenant"])
}

func TestProcessComponentInStackDerivedComponent(t *testing.T) {
	component := "derived-component-3"
	stack := "tenant1-ue2-test-1"

	result, err := ProcessComponentInStack(component, stack, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)

	resultYaml, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if resultYaml != "" {
				t.Logf("Component section:\n%s", resultYaml)
			} else {
				t.Logf("Component section (raw): %+v", result)
			}
		}
	})
}

func TestProcessComponentInStackServiceIamRole(t *testing.T) {
	component := "service-iam-role/webservices/prod"
	stack := "tenant2-ue2-prod"

	result, err := ProcessComponentInStack(component, stack, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)

	resultYaml, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if resultYaml != "" {
				t.Logf("Component section:\n%s", resultYaml)
			} else {
				t.Logf("Component section (raw): %+v", result)
			}
		}
	})
}
