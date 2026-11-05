package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractComponentInfoFromPath_ComponentBaseDiagnostic(t *testing.T) {
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", "")
	t.Setenv("ATMOS_COMPONENTS_HELMFILE_BASE_PATH", "")
	t.Setenv("ATMOS_COMPONENTS_PACKER_BASE_PATH", "")
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root}
	config.Components.Terraform.BasePath = filepath.Join("components", "terraform")
	config.Components.Helmfile.BasePath = filepath.Join("components", "helmfile")
	config.Components.Packer.BasePath = filepath.Join("components", "packer")

	for _, componentType := range []string{"terraform", "helmfile", "packer"} {
		t.Run(componentType, func(t *testing.T) {
			base := filepath.Join(root, "components", componentType)
			require.NoError(t, os.MkdirAll(base, 0o755))

			info, err := ExtractComponentInfoFromPath(config, base)

			require.ErrorIs(t, err, errUtils.ErrPathIsComponentBase)
			assert.NotErrorIs(t, err, errUtils.ErrPathNotInComponentDir)
			assert.Nil(t, info)
		})
	}

	t.Run("outside every base keeps the generic diagnostic", func(t *testing.T) {
		info, err := ExtractComponentInfoFromPath(config, filepath.Join(root, "unrelated"))
		require.ErrorIs(t, err, errUtils.ErrPathNotInComponentDir)
		assert.NotErrorIs(t, err, errUtils.ErrPathIsComponentBase)
		assert.Nil(t, info)
	})
}

func TestExtractComponentInfoFromPath_ComponentBaseAllowsLaterMatch(t *testing.T) {
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", "")
	t.Setenv("ATMOS_COMPONENTS_HELMFILE_BASE_PATH", "")
	t.Setenv("ATMOS_COMPONENTS_PACKER_BASE_PATH", "")
	root := t.TempDir()
	base := filepath.Join(root, "components", "shared")
	require.NoError(t, os.MkdirAll(base, 0o755))
	config := &schema.AtmosConfiguration{BasePath: root}
	config.Components.Terraform.BasePath = filepath.Join("components", "shared")
	config.Components.Helmfile.BasePath = "components"

	info, err := ExtractComponentInfoFromPath(config, base)

	require.NoError(t, err)
	assert.Equal(t, &ComponentInfo{
		ComponentType: "helmfile",
		ComponentName: "shared",
		FullComponent: "shared",
	}, info)
}
