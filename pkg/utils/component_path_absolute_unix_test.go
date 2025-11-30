//go:build !windows
// +build !windows

package utils

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetComponentPath_AllComponentTypes tests all component types (terraform, helmfile, packer) on Unix systems.
func TestGetComponentPath_AllComponentTypes(t *testing.T) {
	basePath := "/home/runner/_work/infrastructure/infrastructure"

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 basePath,
		TerraformDirAbsolutePath: filepath.Join(basePath, "atmos", "components", "terraform"),
		HelmfileDirAbsolutePath:  filepath.Join(basePath, "atmos", "components", "helmfile"),
		PackerDirAbsolutePath:    filepath.Join(basePath, "atmos", "components", "packer"),
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "atmos/components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "atmos/components/helmfile",
			},
			Packer: schema.Packer{
				BasePath: "atmos/components/packer",
			},
		},
	}

	componentTypes := []struct {
		componentType string
		component     string
		expectedPath  string
	}{
		{
			componentType: "terraform",
			component:     "iam-role",
			expectedPath:  "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform/iam-role",
		},
		{
			componentType: "helmfile",
			component:     "nginx",
			expectedPath:  "/home/runner/_work/infrastructure/infrastructure/atmos/components/helmfile/nginx",
		},
		{
			componentType: "packer",
			component:     "ami-builder",
			expectedPath:  "/home/runner/_work/infrastructure/infrastructure/atmos/components/packer/ami-builder",
		},
	}

	for _, ct := range componentTypes {
		t.Run(ct.componentType, func(t *testing.T) {
			componentPath, err := GetComponentPath(
				atmosConfig,
				ct.componentType,
				"", // No folder prefix.
				ct.component,
			)

			require.NoError(t, err)
			assert.Equal(t, ct.expectedPath, componentPath,
				"Component path should match expected for %s", ct.componentType)

			// Ensure no duplication.
			assert.NotContains(t, componentPath, "/.//",
				"%s component path should not contain /.//", ct.componentType)
			assert.NotContains(t, componentPath,
				"/home/runner/_work/infrastructure/infrastructure/home/runner/_work/infrastructure/infrastructure",
				"%s component path should not have path duplication", ct.componentType)
		})
	}
}
