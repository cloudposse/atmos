package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosio "github.com/cloudposse/atmos/pkg/io"
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

// TestConstructTerraformComponentVarfileName tests the non-Clean-suffixed varfile
// name export used by other packages (e.g. tfmigrate's buildTfmigrateEnv), which
// must produce the same name as the terraform execution path itself.
func TestConstructTerraformComponentVarfileName(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		Component:     "vpc",
		ContextPrefix: "ue1-dev",
	}

	result := ConstructTerraformComponentVarfileName(info)

	assert.Equal(t, "ue1-dev-vpc.terraform.tfvars.json", result)
	assert.Equal(t, ConstructTerraformComponentVarfileNameForClean(info), result,
		"the tfmigrate-facing export and the clean-facing export must construct identical names")
}

// TestComputeTerraformSecretVarEnv exercises the exported tfmigrate-facing wrapper
// end-to-end: it must both flag secret-bearing keys on info (the same side effect
// computeTerraformSecretVarKeys has directly) and return the matching TF_VAR_ env
// entries, so callers that only have access to the exported wrapper (outside this
// package) still get real secret partitioning behavior.
func TestComputeTerraformSecretVarEnv(t *testing.T) {
	const secret = "tf-adapter-SECRET-xyz789"
	atmosio.RegisterSecret(secret)

	info := &schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{
			"db_password": secret,
			"region":      "us-east-1-adapter",
		},
	}

	env, err := ComputeTerraformSecretVarEnv(info)
	require.NoError(t, err)

	require.NotNil(t, info.TerraformSecretVarKeys)
	assert.True(t, info.TerraformSecretVarKeys["db_password"], "secret-bearing key must be flagged as a side effect")
	assert.False(t, info.TerraformSecretVarKeys["region"], "non-secret key must not be flagged")

	joined := strings.Join(env, "\n")
	assert.Contains(t, joined, "TF_VAR_db_password="+secret)
	assert.NotContains(t, joined, "TF_VAR_region=", "non-secret vars must not be injected as env entries")
}

// TestComputeTerraformSecretVarEnv_NoSecrets confirms the wrapper returns an empty
// env slice (not an error) when nothing in the component vars is secret.
func TestComputeTerraformSecretVarEnv_NoSecrets(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{"region": "us-west-2-adapter-nosecret"},
	}

	env, err := ComputeTerraformSecretVarEnv(info)
	require.NoError(t, err)
	assert.Empty(t, env)
	assert.Nil(t, info.TerraformSecretVarKeys)
}
