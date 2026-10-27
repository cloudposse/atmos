package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

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

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "existing component should not return error")
}

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

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err, "missing component without source should return error")
	assert.Contains(t, err.Error(), "nonexistent")
}

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
	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "component with workdir path set should pass")
}

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

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "component with folder prefix should be found")
}

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

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing-comp")
	assert.Contains(t, err.Error(), filepath.Join("components", "terraform"))
}

func TestExecuteTerraformGenerateVarfileCmd_Deprecated(t *testing.T) {
	err := ExecuteTerraformGenerateVarfileCmd(nil, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDeprecatedCmdNotCallable)
}

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

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponent)
	assert.Contains(t, err.Error(), "auto-provision")
}

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
	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err, "should fail when component doesn't exist")

	// Now create the directory to simulate JIT provisioning having put it there.
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte("# lazy-vpc\n"), 0o644))

	// Second call: component now exists at the standard path.
	err = EnsureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "should pass when component exists")
}

func TestVarfileVarsToWrite(t *testing.T) {
	const secret = "genvarfile-secret-aa11bb22"
	iolib.RegisterSecret(secret)

	newInfo := func() *schema.ConfigAndStacksInfo {
		return &schema.ConfigAndStacksInfo{
			ComponentVarsSection: map[string]any{
				"db_password": secret,
				"region":      "us-east-1-genvarfile",
			},
		}
	}

	t.Run("default omits secrets", func(t *testing.T) {
		info := newInfo()
		got := VarfileVarsToWrite(info, false, "test.tfvars.json")
		_, hasSecret := got["db_password"]
		assert.False(t, hasSecret, "secret var must be omitted from the varfile by default")
		assert.Equal(t, "us-east-1-genvarfile", got["region"], "non-secret var must remain")
	})

	t.Run("with-secrets includes secrets", func(t *testing.T) {
		info := newInfo()
		got := VarfileVarsToWrite(info, true, "test.tfvars.json")
		assert.Equal(t, secret, got["db_password"], "secret var must be written with --with-secrets")
		assert.Equal(t, "us-east-1-genvarfile", got["region"])
	})
}
