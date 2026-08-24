package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDescribeStacks_TemplateEnvVarPropagation verifies that env vars configured in
// atmos.yaml templates.settings.env and stack manifest settings.templates.settings.env
// are properly propagated to template processing during ExecuteDescribeStacks.
// This is an integration test for issue #2083.
func TestDescribeStacks_TemplateEnvVarPropagation(t *testing.T) {
	// Use t.Setenv for automatic restore on test cleanup, then unset for clean state.
	t.Setenv("TEST_TMPL_ENV_VAR", "")
	os.Unsetenv("TEST_TMPL_ENV_VAR")
	t.Setenv("TEST_TMPL_STACK_VAR", "")
	os.Unsetenv("TEST_TMPL_STACK_VAR")

	testDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "template-env-vars")
	t.Chdir(testDir)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Verify that templates.settings.env was loaded from atmos.yaml.
	require.NotEmpty(t, atmosConfig.Templates.Settings.Env, "templates.settings.env should be loaded from atmos.yaml")
	assert.Equal(t, "from-atmos-yaml", atmosConfig.Templates.Settings.Env["TEST_TMPL_ENV_VAR"],
		"atmos.yaml should have TEST_TMPL_ENV_VAR set")

	// Run ExecuteDescribeStacks with processTemplates=true.
	result, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",         // filterByStack
		[]string{}, // components
		[]string{}, // componentTypes
		[]string{}, // sections
		false,      // ignoreMissingFiles
		true,       // processTemplates - this exercises the env var code path
		false,      // processYamlFunctions
		false,      // includeEmptyStacks
		[]string{}, // skip
		nil,        // authManager
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Find the "test" stack (from stacks.name_template: "{{ .vars.stage }}").
	stackData, ok := result["test"].(map[string]any)
	require.True(t, ok, "Stack 'test' should exist in result")

	components, ok := stackData["components"].(map[string]any)
	require.True(t, ok, "Stack should have components")

	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok, "Stack should have terraform components")

	envTest, ok := terraform["env-test"].(map[string]any)
	require.True(t, ok, "Component 'env-test' should exist")

	vars, ok := envTest["vars"].(map[string]any)
	require.True(t, ok, "Component should have vars")

	// Verify env vars from both atmos.yaml and stack manifest were available in the template.
	fooVal, ok := vars["foo"].(string)
	require.True(t, ok, "vars.foo should be a string")

	assert.Contains(t, fooVal, "from-atmos-yaml",
		"env var from atmos.yaml (TEST_TMPL_ENV_VAR) should be in template output")
	assert.Contains(t, fooVal, "from-stack-manifest",
		"env var from stack manifest (TEST_TMPL_STACK_VAR) should be in template output")
	assert.Equal(t, "from-atmos-yaml-from-stack-manifest", fooVal,
		"template should combine both env vars")

	// Verify env vars are cleaned up after processing.
	_, exists := os.LookupEnv("TEST_TMPL_ENV_VAR")
	assert.False(t, exists, "TEST_TMPL_ENV_VAR should be unset after processing")

	_, exists = os.LookupEnv("TEST_TMPL_STACK_VAR")
	assert.False(t, exists, "TEST_TMPL_STACK_VAR should be unset after processing")
}

// TestInitCliConfig_TemplateEnvCaseSensitive verifies that templates.settings.env keys
// preserve their original case through Viper config loading.
func TestInitCliConfig_TemplateEnvCaseSensitive(t *testing.T) {
	testDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "template-env-vars")
	t.Chdir(testDir)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Verify the env var key preserved its original case (not lowercased by Viper).
	envMap := atmosConfig.Templates.Settings.Env
	require.NotEmpty(t, envMap, "templates.settings.env should not be empty")

	// Check that the key is TEST_TMPL_ENV_VAR (uppercase) not test_tmpl_env_var.
	_, hasUppercase := envMap["TEST_TMPL_ENV_VAR"]
	assert.True(t, hasUppercase, "Env var key should preserve uppercase: TEST_TMPL_ENV_VAR")

	// Also verify via GetCaseSensitiveMap.
	caseSensitiveEnv := atmosConfig.GetCaseSensitiveMap("templates.settings.env")
	require.NotEmpty(t, caseSensitiveEnv, "GetCaseSensitiveMap should return env map")
	assert.Equal(t, "from-atmos-yaml", caseSensitiveEnv["TEST_TMPL_ENV_VAR"],
		"case-sensitive map should have the correct value")
}
