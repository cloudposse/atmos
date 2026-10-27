package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cfg "github.com/cloudposse/atmos/pkg/config"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestEnsureTerraformComponentExists_ExistingComponent tests that existing components pass validation.

// TestEnsureTerraformComponentExists_MissingComponentNoSource tests error for missing component without source.

// TestEnsureTerraformComponentExists_WorkdirPathSet tests that workdir path set by provisioner is accepted.

// TestEnsureTerraformComponentExists_WithFolderPrefix tests component resolution with a folder prefix.

// TestEnsureTerraformComponentExists_ReturnsErrorWithBasePath tests the error message contains base path info.

// TestExecuteTerraformGenerateVarfileCmd_Deprecated tests the deprecated command returns an error.

// TestEnsureTerraformComponentExists_JITProvisionFails tests the error wrapping when JIT provisioning fails.

// TestEnsureTerraformComponentExists_PostJITComponentAppears tests the path where JIT provisioning
// succeeds (no source, returns nil) and the component directory appears at the standard path.

// TestExecuteGenerateVarfile_ProcessStacksFails tests that ExecuteGenerateVarfile returns an error
// when ProcessStacks fails due to missing stack config files.
func TestExecuteGenerateVarfile_ProcessStacksFails(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	opts := &VarfileOptions{
		Component: "vpc",
		Stack:     "dev",
	}

	err := ExecuteGenerateVarfile(opts, atmosConfig)
	assert.Error(t, err, "should fail when stack config is not set up")
}

// TestExecuteGenerateVarfile_ProcessStacksFailsWithFile tests the file option path is not reached on ProcessStacks failure.
func TestExecuteGenerateVarfile_ProcessStacksFailsWithFile(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	opts := &VarfileOptions{
		Component: "vpc",
		Stack:     "dev",
		File:      filepath.Join(tempDir, "output.tfvars.json"),
		ProcessingOptions: ProcessingOptions{
			ProcessTemplates: true,
			ProcessFunctions: true,
		},
	}

	err := ExecuteGenerateVarfile(opts, atmosConfig)
	assert.Error(t, err, "should fail when stack config is not set up")
}

// TestExecuteGenerateVarfile_Integration tests the full varfile generation flow
// using the existing stack-templates test fixture.
func TestExecuteGenerateVarfile_Integration(t *testing.T) {
	fixtureDir, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "stack-templates"))
	require.NoError(t, err)

	// Skip if the fixture directory doesn't exist.
	if _, statErr := os.Stat(fixtureDir); os.IsNotExist(statErr) {
		t.Skip("Stack-templates fixture not found")
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		AtmosBasePath:          fixtureDir,
		AtmosConfigDirsFromArg: []string{fixtureDir},
	}, true)
	require.NoError(t, err, "config initialization should succeed")

	// Write the varfile to a temp directory to avoid modifying the fixture.
	varfilePath := filepath.Join(t.TempDir(), "test-output.tfvars.json")
	opts := &VarfileOptions{
		Component: "component-1",
		Stack:     "nonprod",
		File:      varfilePath,
		ProcessingOptions: ProcessingOptions{
			ProcessTemplates: true,
			ProcessFunctions: true,
		},
	}

	err = ExecuteGenerateVarfile(opts, &atmosConfig)
	require.NoError(t, err, "varfile generation should succeed")

	// Verify the varfile was written.
	assert.FileExists(t, varfilePath)
	content, err := os.ReadFile(varfilePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "component-1-a")
}

// TestExecuteGenerateBackend_Integration tests the full backend generation flow
// using the existing stack-templates test fixture.
func TestExecuteGenerateBackend_Integration(t *testing.T) {
	fixtureDir, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "stack-templates"))
	require.NoError(t, err)

	// Skip if the fixture directory doesn't exist.
	if _, statErr := os.Stat(fixtureDir); os.IsNotExist(statErr) {
		t.Skip("Stack-templates fixture not found")
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		AtmosBasePath:          fixtureDir,
		AtmosConfigDirsFromArg: []string{fixtureDir},
	}, true)
	require.NoError(t, err, "config initialization should succeed")

	opts := &GenerateBackendOptions{
		Component: "component-1",
		Stack:     "nonprod",
		ProcessingOptions: ProcessingOptions{
			ProcessTemplates: true,
			ProcessFunctions: true,
		},
	}

	err = ExecuteGenerateBackend(opts, &atmosConfig)
	require.NoError(t, err, "backend generation should succeed")

	// Verify backend was written to the component directory.
	backendFile := filepath.Join(fixtureDir, "components", "terraform", "mock", "backend.tf.json")
	t.Cleanup(func() { _ = os.Remove(backendFile) })
	assert.FileExists(t, backendFile)
	content, err := os.ReadFile(backendFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "nonprod-tfstate")
}

// TestExecuteGenerateVarfile_SensitiveStateValueNeverLeaks reproduces the sensitive
// YAML-function regression end-to-end. The explicit YAML tag must survive decoding,
// the state lookup must be evaluated, and the sensitive consumer input must use
// TF_VAR_ instead of the generated tfvars JSON file.
func TestExecuteGenerateVarfile_SensitiveStateValueNeverLeaks(t *testing.T) {
	iolib.Reset()
	t.Cleanup(iolib.Reset)
	require.NoError(t, iolib.Initialize())
	iolib.GetContext().Masker().SetEnabled(true)

	fixtureDir, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "yaml-sensitive-state-varfile"))
	require.NoError(t, err)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		AtmosBasePath:          fixtureDir,
		AtmosConfigDirsFromArg: []string{fixtureDir},
	}, true)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalStateGetter := stateGetter
	stateGetter = mockStateGetter
	t.Cleanup(func() { stateGetter = originalStateGetter })

	const function = `!terraform.state producer ".fields.PRIVATE_KEY"`
	const resolved = "demo-secret-value"
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), function, "dev", "producer", ".fields.PRIVATE_KEY", false, gomock.Any(), gomock.Any()).
		Return(resolved, nil).
		AnyTimes()

	varfilePath := filepath.Join(t.TempDir(), "consumer.tfvars.json")
	err = ExecuteGenerateVarfile(&VarfileOptions{
		Component: "consumer",
		Stack:     "dev",
		File:      varfilePath,
		ProcessingOptions: ProcessingOptions{
			ProcessTemplates: true,
			ProcessFunctions: true,
		},
	}, &atmosConfig)
	require.NoError(t, err)

	contents, err := os.ReadFile(varfilePath)
	require.NoError(t, err)
	assert.NotContains(t, string(contents), "sensitive_value")
	assert.NotContains(t, string(contents), resolved)
	assert.NotContains(t, string(contents), "producer")
	assert.NotContains(t, string(contents), "PRIVATE_KEY")

	var written map[string]any
	require.NoError(t, json.Unmarshal(contents, &written))
	assert.Equal(t, "ordinary-value", written["ordinary_value"])

	// Re-process once to inspect the exact transient environment handoff without
	// relying on a Terraform executable. This confirms the resolved value, not
	// the lookup arguments, becomes TF_VAR_sensitive_value.
	info, err := ProcessStacks(&atmosConfig, schema.ConfigAndStacksInfo{
		ComponentFromArg: "consumer",
		Stack:            "dev",
		StackFromArg:     "dev",
		ComponentType:    cfg.TerraformComponentType,
	}, true, true, true, nil, nil)
	require.NoError(t, err)
	computeTerraformSecretVarKeys(&info)
	env, err := secretVarEnv(&info)
	require.NoError(t, err)
	joinedEnv := strings.Join(env, "\n")
	assert.Contains(t, joinedEnv, "TF_VAR_sensitive_value="+resolved)
	assert.NotContains(t, joinedEnv, "producer")
	assert.NotContains(t, joinedEnv, "PRIVATE_KEY")
	assert.NotContains(t, joinedEnv, function)
}
