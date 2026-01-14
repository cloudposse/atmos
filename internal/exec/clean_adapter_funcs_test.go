package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCollectComponentsDirectoryObjectsForClean tests that the function delegates correctly.
func TestCollectComponentsDirectoryObjectsForClean(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test component directory.
	vpcDir := filepath.Join(tempDir, "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))

	// Create files to match.
	lockFile := filepath.Join(vpcDir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile, []byte("lock"), 0o644))

	// Call the adapter function.
	folders, err := CollectComponentsDirectoryObjectsForClean(tempDir, []string{"vpc"}, []string{".terraform.lock.hcl"})
	require.NoError(t, err)

	// Verify results.
	assert.Len(t, folders, 1)
	assert.Len(t, folders[0].Files, 1)
	assert.Equal(t, ".terraform.lock.hcl", folders[0].Files[0].Name)
}

// TestCollectComponentsDirectoryObjectsForClean_EmptyPath tests error handling.
func TestCollectComponentsDirectoryObjectsForClean_EmptyPath(t *testing.T) {
	_, err := CollectComponentsDirectoryObjectsForClean("", []string{"vpc"}, []string{".terraform"})
	require.Error(t, err)
}

// TestGetAllStacksComponentsPathsForClean tests that the function delegates correctly.
func TestGetAllStacksComponentsPathsForClean(t *testing.T) {
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc-dev": map[string]any{
						"component": "vpc",
					},
					"rds-dev": map[string]any{
						"component": "rds",
					},
				},
			},
		},
		"staging": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc-staging": map[string]any{
						"component": "vpc", // Same component as dev.
					},
				},
			},
		},
	}

	paths := GetAllStacksComponentsPathsForClean(stacksMap)

	// Should deduplicate paths.
	assert.Len(t, paths, 2) // vpc, rds
	assert.Contains(t, paths, "vpc")
	assert.Contains(t, paths, "rds")
}

// TestGetAllStacksComponentsPathsForClean_EmptyMap tests with empty input.
func TestGetAllStacksComponentsPathsForClean_EmptyMap(t *testing.T) {
	paths := GetAllStacksComponentsPathsForClean(map[string]any{})
	assert.Nil(t, paths)
}

// TestConstructTerraformComponentVarfileNameForClean tests varfile name construction.
func TestConstructTerraformComponentVarfileNameForClean(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		Component:     "vpc",
		ContextPrefix: "ue1-dev",
	}

	result := ConstructTerraformComponentVarfileNameForClean(info)

	// The result should be formatted as ContextPrefix-Component.terraform.tfvars.json.
	assert.Equal(t, "ue1-dev-vpc.terraform.tfvars.json", result)
}

// TestConstructTerraformComponentVarfileNameForClean_WithFolderPrefix tests with folder prefix.
func TestConstructTerraformComponentVarfileNameForClean_WithFolderPrefix(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		Component:                     "vpc",
		ContextPrefix:                 "ue1-dev",
		ComponentFolderPrefixReplaced: "networking",
	}

	result := ConstructTerraformComponentVarfileNameForClean(info)

	// The result should include the folder prefix.
	assert.Equal(t, "ue1-dev-networking-vpc.terraform.tfvars.json", result)
}

// TestConstructTerraformComponentPlanfileNameForClean tests planfile name construction.
func TestConstructTerraformComponentPlanfileNameForClean(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		Component:     "vpc",
		ContextPrefix: "ue1-dev",
	}

	result := ConstructTerraformComponentPlanfileNameForClean(info)

	// The result should be formatted as ContextPrefix-Component.planfile.
	assert.Equal(t, "ue1-dev-vpc.planfile", result)
}

// TestConstructTerraformComponentPlanfileNameForClean_WithFolderPrefix tests with folder prefix.
func TestConstructTerraformComponentPlanfileNameForClean_WithFolderPrefix(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		Component:                     "vpc",
		ContextPrefix:                 "ue1-dev",
		ComponentFolderPrefixReplaced: "networking",
	}

	result := ConstructTerraformComponentPlanfileNameForClean(info)

	// The result should include the folder prefix.
	assert.Equal(t, "ue1-dev-networking-vpc.planfile", result)
}
