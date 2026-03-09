package describe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	u "github.com/cloudposse/atmos/pkg/utils"
)

func logOnFailure(t *testing.T, result map[string]any) {
	t.Helper()
	resultYaml, _ := u.ConvertToYAML(result)
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

func TestProcessComponentInStack(t *testing.T) {
	component := "infra/vpc"
	stack := "tenant1-ue2-dev"

	result, err := ProcessComponentInStack(component, stack, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	logOnFailure(t, result)
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

func TestProcessComponentInStackErrorCases(t *testing.T) {
	tests := []struct {
		name      string
		component string
		stack     string
	}{
		{"invalid component", "nonexistent-component", "tenant1-ue2-dev"},
		{"invalid stack", "infra/vpc", "nonexistent-stack"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ProcessComponentInStack(tt.component, tt.stack, "", "")
			assert.Error(t, err)
		})
	}
}

func TestProcessComponentFromContext(t *testing.T) {
	result, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:   "infra/vpc",
		Tenant:      "tenant1",
		Environment: "ue2",
		Stage:       "dev",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	logOnFailure(t, result)
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

func TestProcessComponentFromContextNilParams(t *testing.T) {
	_, err := ProcessComponentFromContext(nil)
	assert.Error(t, err)
}

func TestProcessComponentFromContextErrorCases(t *testing.T) {
	tests := []struct {
		name   string
		params *ComponentFromContextParams
	}{
		{
			"invalid component",
			&ComponentFromContextParams{
				Component:   "nonexistent-component",
				Tenant:      "tenant1",
				Environment: "ue2",
				Stage:       "dev",
			},
		},
		{
			"invalid context",
			&ComponentFromContextParams{
				Component:   "infra/vpc",
				Tenant:      "nonexistent",
				Environment: "xxx",
				Stage:       "yyy",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ProcessComponentFromContext(tt.params)
			assert.Error(t, err)
		})
	}
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
	logOnFailure(t, result)
}

func TestProcessComponentInStackServiceIamRole(t *testing.T) {
	component := "service-iam-role/webservices/prod"
	stack := "tenant2-ue2-prod"

	result, err := ProcessComponentInStack(component, stack, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	logOnFailure(t, result)
}

// Tests using the locals-logical-names fixture which uses stacks.name_template
// instead of stacks.name_pattern. This exercises the stackNameTemplate branch
// in ProcessComponentFromContext.

func TestProcessComponentInStackWithNameTemplate(t *testing.T) {
	atmosCliConfigPath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "locals-logical-names")
	component := "vpc"
	stack := "dev-us-east-1"

	result, err := ProcessComponentInStack(component, stack, atmosCliConfigPath, "")
	require.NoError(t, err)
	require.NotNil(t, result)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "result should contain 'vars' section")
	assert.Equal(t, "dev", vars["environment"])
	assert.Equal(t, "us-east-1", vars["stage"])
}

func TestProcessComponentFromContextWithNameTemplate(t *testing.T) {
	atmosCliConfigPath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "locals-logical-names")

	result, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:          "vpc",
		Environment:        "dev",
		Stage:              "us-east-1",
		AtmosCliConfigPath: atmosCliConfigPath,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "result should contain 'vars' section")
	assert.Equal(t, "dev", vars["environment"])
	assert.Equal(t, "us-east-1", vars["stage"])
}

func TestProcessComponentFromContextWithNameTemplateInvalidContext(t *testing.T) {
	atmosCliConfigPath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "locals-logical-names")

	_, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:          "vpc",
		Environment:        "nonexistent",
		Stage:              "xxx",
		AtmosCliConfigPath: atmosCliConfigPath,
	})
	assert.Error(t, err)
}

func TestProcessComponentFromContextMatchesStackWithNameTemplate(t *testing.T) {
	atmosCliConfigPath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "locals-logical-names")

	resultByStack, err := ProcessComponentInStack("vpc", "prod-us-west-2", atmosCliConfigPath, "")
	require.NoError(t, err)

	resultByContext, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:          "vpc",
		Environment:        "prod",
		Stage:              "us-west-2",
		AtmosCliConfigPath: atmosCliConfigPath,
	})
	require.NoError(t, err)

	resultByStackYaml, err := u.ConvertToYAML(resultByStack)
	require.NoError(t, err)
	resultByContextYaml, err := u.ConvertToYAML(resultByContext)
	require.NoError(t, err)

	assert.Equal(t, resultByStackYaml, resultByContextYaml,
		"ProcessComponentInStack and ProcessComponentFromContext should return the same result with name_template")
}

func TestProcessComponentFromContextNoNameConfig(t *testing.T) {
	tmpDir := t.TempDir()

	atmosYaml := []byte(`base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
`)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), atmosYaml, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "components", "terraform"), 0o755))

	_, err := ProcessComponentFromContext(&ComponentFromContextParams{
		Component:          "test",
		Environment:        "dev",
		Stage:              "us-east-1",
		AtmosCliConfigPath: tmpDir,
		AtmosBasePath:      tmpDir,
	})
	assert.Error(t, err)
}
