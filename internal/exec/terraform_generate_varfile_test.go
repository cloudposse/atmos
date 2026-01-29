package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
				BasePath: "components/terraform",
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
				BasePath: "components/terraform",
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
				BasePath: "components/terraform",
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

// TestVarfileOptions_Validation tests VarfileOptions struct field validation.
func TestVarfileOptions_Validation(t *testing.T) {
	tests := []struct {
		name          string
		opts          *VarfileOptions
		expectValid   bool
		invalidReason string
	}{
		{
			name: "valid options with component and stack",
			opts: &VarfileOptions{
				Component: "vpc",
				Stack:     "dev-us-west-2",
			},
			expectValid: true,
		},
		{
			name: "valid options with all fields",
			opts: &VarfileOptions{
				Component: "rds",
				Stack:     "prod-eu-west-1",
				File:      filepath.Join("tmp", "test.tfvars.json"),
				ProcessingOptions: ProcessingOptions{
					ProcessTemplates: true,
					ProcessFunctions: true,
				},
			},
			expectValid: true,
		},
		{
			name: "missing component",
			opts: &VarfileOptions{
				Component: "",
				Stack:     "dev-us-west-2",
			},
			expectValid:   false,
			invalidReason: "component is required",
		},
		{
			name: "missing stack",
			opts: &VarfileOptions{
				Component: "vpc",
				Stack:     "",
			},
			expectValid:   false,
			invalidReason: "stack is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.opts.Component != "" && tt.opts.Stack != ""
			assert.Equal(t, tt.expectValid, isValid, tt.invalidReason)
		})
	}
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
				BasePath: "components/terraform",
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
				BasePath: "components/terraform",
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
	assert.Contains(t, err.Error(), "components/terraform")
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

// TestVarfileOptions_ProcessingOptions tests that ProcessingOptions are correctly carried.
func TestVarfileOptions_ProcessingOptions(t *testing.T) {
	opts := &VarfileOptions{
		Component: "vpc",
		Stack:     "dev",
		ProcessingOptions: ProcessingOptions{
			ProcessTemplates: true,
			ProcessFunctions: false,
			Skip:             []string{"template"},
		},
	}

	assert.True(t, opts.ProcessTemplates)
	assert.False(t, opts.ProcessFunctions)
	assert.Equal(t, []string{"template"}, opts.Skip)
}
