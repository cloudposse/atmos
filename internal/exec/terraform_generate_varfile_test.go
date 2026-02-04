package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestEnsureTerraformComponentExists_ExistingComponent tests that existing components pass validation.
func TestEnsureTerraformComponentExists_ExistingComponent(t *testing.T) {
	// Create a temporary directory structure.
	tempDir := t.TempDir()
	componentPath := filepath.Join(tempDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))

	// Create a minimal main.tf to make it a valid component.
	mainTF := filepath.Join(componentPath, "main.tf")
	require.NoError(t, os.WriteFile(mainTF, []byte("# vpc component\n"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "vpc",
		FinalComponent:        "vpc",
		ComponentFolderPrefix: "",
		ComponentSection:      map[string]any{},
	}

	err := ensureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "existing component should not return error")
}

// TestEnsureTerraformComponentExists_MissingComponentNoSource tests error for missing component without source.
func TestEnsureTerraformComponentExists_MissingComponentNoSource(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "nonexistent",
		FinalComponent:        "nonexistent",
		ComponentFolderPrefix: "",
		ComponentSection:      map[string]any{},
	}

	err := ensureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err, "missing component without source should return error")
	assert.Contains(t, err.Error(), "nonexistent")
}

// TestEnsureTerraformComponentExists_WorkdirPathSet tests that workdir path set by provisioner is accepted.
func TestEnsureTerraformComponentExists_WorkdirPathSet(t *testing.T) {
	tempDir := t.TempDir()

	// Create workdir path.
	workdirPath := filepath.Join(tempDir, "workdir", "vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "vpc",
		FinalComponent:        "vpc",
		ComponentFolderPrefix: "",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		},
	}

	// Even though the original component path doesn't exist, the workdir path is set.
	err := ensureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "component with workdir path set should pass")
}

// TestTryJITProvision_NoSource tests that tryJITProvision returns nil when no source is configured.
func TestTryJITProvision_NoSource(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}

	err := tryJITProvision(atmosConfig, info)
	assert.NoError(t, err, "no source should return nil without error")
}

// TestTryJITProvision_WithEmptySource tests that empty source config is handled.
func TestTryJITProvision_WithEmptySource(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"source": map[string]any{},
		},
	}

	err := tryJITProvision(atmosConfig, info)
	assert.NoError(t, err, "empty source should return nil without error")
}

// TestEnsureTerraformComponentExists_WithFolderPrefix tests component resolution with a folder prefix.
func TestEnsureTerraformComponentExists_WithFolderPrefix(t *testing.T) {
	tempDir := t.TempDir()
	componentPath := filepath.Join(tempDir, "components", "terraform", "myprefix", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte("# vpc\n"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "vpc",
		FinalComponent:        "vpc",
		ComponentFolderPrefix: "myprefix",
		ComponentSection:      map[string]any{},
	}

	err := ensureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "component with folder prefix should be found")
}

// TestEnsureTerraformComponentExists_ReturnsErrorWithBasePath tests the error message contains base path info.
func TestEnsureTerraformComponentExists_ReturnsErrorWithBasePath(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "missing-comp",
		FinalComponent:        "missing-comp",
		ComponentFolderPrefix: "",
		ComponentSection:      map[string]any{},
	}

	err := ensureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing-comp")
	assert.Contains(t, err.Error(), filepath.Join("components", "terraform"))
}

// TestTryJITProvision_NilComponentSection tests that tryJITProvision handles nil ComponentSection.
func TestTryJITProvision_NilComponentSection(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: nil,
	}

	err := tryJITProvision(atmosConfig, info)
	assert.NoError(t, err, "nil component section should return nil without error")
}

// TestTryJITProvision_WithNonSourceKeys tests that component sections without source are handled.
func TestTryJITProvision_WithNonSourceKeys(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"vars":     map[string]any{"name": "test"},
			"settings": map[string]any{"enabled": true},
			"metadata": map[string]any{"component": "vpc"},
		},
	}

	err := tryJITProvision(atmosConfig, info)
	assert.NoError(t, err, "section without source should return nil without error")
}

// TestTryJITProvision_WithSourceURI tests that tryJITProvision exercises AutoProvisionSource
// when a valid source URI is configured (but fails because the URI is unreachable).
func TestTryJITProvision_WithSourceURI(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"source": map[string]any{
				"uri": "file:///nonexistent/path/to/source",
			},
		},
	}

	err := tryJITProvision(atmosConfig, info)
	// Should return an error because the source URI is unreachable.
	assert.Error(t, err, "should fail when source URI is unreachable")
	assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponent)
}

// TestCheckDirectoryExists tests all branches of the checkDirectoryExists function.
func TestCheckDirectoryExists(t *testing.T) {
	t.Run("existing directory returns true", func(t *testing.T) {
		tempDir := t.TempDir()
		exists, err := checkDirectoryExists(tempDir)
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("non-existing directory returns false", func(t *testing.T) {
		exists, err := checkDirectoryExists(filepath.Join(t.TempDir(), "nonexistent"))
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("file path returns false", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "file.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("test"), 0o644))
		exists, err := checkDirectoryExists(filePath)
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

// TestExecuteTerraformGenerateVarfileCmd_Deprecated tests the deprecated command returns an error.
func TestExecuteTerraformGenerateVarfileCmd_Deprecated(t *testing.T) {
	err := ExecuteTerraformGenerateVarfileCmd(nil, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDeprecatedCmdNotCallable)
}

// TestEnsureTerraformComponentExists_JITProvisionFails tests the error wrapping when JIT provisioning fails.
func TestEnsureTerraformComponentExists_JITProvisionFails(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	// Component doesn't exist and has a source with an unreachable URI — JIT provision will fail.
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "jit-fail",
		FinalComponent:        "jit-fail",
		ComponentFolderPrefix: "",
		ComponentSection: map[string]any{
			"source": map[string]any{
				"uri": "file:///nonexistent/path/to/source",
			},
		},
	}

	err := ensureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponent)
	assert.Contains(t, err.Error(), "auto-provision")
}

// TestEnsureTerraformComponentExists_PostJITComponentAppears tests the path where JIT provisioning
// succeeds (no source, returns nil) and the component directory appears at the standard path.
func TestEnsureTerraformComponentExists_PostJITComponentAppears(t *testing.T) {
	tempDir := t.TempDir()

	// Do NOT create the component directory initially.
	componentPath := filepath.Join(tempDir, "components", "terraform", "lazy-vpc")

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "lazy-vpc",
		FinalComponent:        "lazy-vpc",
		ComponentFolderPrefix: "",
		ComponentSection:      map[string]any{},
	}

	// First call: component doesn't exist, no source, no workdir → error.
	err := ensureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err, "should fail when component doesn't exist")

	// Now create the directory to simulate JIT provisioning having put it there.
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte("# lazy-vpc\n"), 0o644))

	// Second call: component now exists at the standard path.
	err = ensureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "should pass when component exists")
}

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
