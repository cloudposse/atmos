package scanners

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestComponentPathNilInputsFallBackToWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	assert.Equal(t, wd, ComponentPath(nil))
	assert.Equal(t, wd, ComponentPath(&Context{}))
	assert.Equal(t, wd, ComponentPath(&Context{AtmosConfig: &schema.AtmosConfiguration{}}))
	assert.Equal(t, wd, ComponentPath(&Context{Info: &schema.ConfigAndStacksInfo{}}))
}

func TestComponentPathUsesProvisionedWorkdirWhenItExists(t *testing.T) {
	base := t.TempDir()
	workdirRoot := filepath.Join(base, ".workdir", cfg.TerraformComponentType, "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirRoot, 0o755))

	ctx := &Context{
		AtmosConfig: &schema.AtmosConfiguration{BasePath: base},
		Info:        &schema.ConfigAndStacksInfo{FinalComponent: "vpc", Stack: "dev"},
	}

	assert.Equal(t, workdirRoot, ComponentPath(ctx))
}

func TestComponentPathFallsBackToWorkingDirectoryWhenBaseEmpty(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	// No provisioned workdir exists and TerraformDirAbsolutePath is empty, so
	// ComponentPath falls all the way back to the process working directory.
	ctx := &Context{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{FinalComponent: "vpc", Stack: "dev"},
	}
	assert.Equal(t, wd, ComponentPath(ctx))
}

func TestComponentPathFallsBackToTerraformDirWithComponentPrecedence(t *testing.T) {
	base := t.TempDir()
	terraformDir := filepath.Join(base, "components", "terraform")

	tests := []struct {
		name string
		info *schema.ConfigAndStacksInfo
		want string
	}{
		{
			name: "prefers FinalComponent",
			info: &schema.ConfigAndStacksInfo{
				FinalComponent:   "final-vpc",
				ComponentFromArg: "arg-vpc",
				Component:        "plain-vpc",
			},
			want: filepath.Join(terraformDir, "final-vpc"),
		},
		{
			name: "falls back to ComponentFromArg when FinalComponent empty",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "arg-vpc",
				Component:        "plain-vpc",
			},
			want: filepath.Join(terraformDir, "arg-vpc"),
		},
		{
			name: "falls back to Component when both others empty",
			info: &schema.ConfigAndStacksInfo{
				Component: "plain-vpc",
			},
			want: filepath.Join(terraformDir, "plain-vpc"),
		},
		{
			name: "honors ComponentFolderPrefix",
			info: &schema.ConfigAndStacksInfo{
				Component:             "plain-vpc",
				ComponentFolderPrefix: "prefix",
			},
			want: filepath.Join(terraformDir, "prefix", "plain-vpc"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				AtmosConfig: &schema.AtmosConfiguration{
					BasePath:                 base,
					TerraformDirAbsolutePath: terraformDir,
				},
				Info: tt.info,
			}
			assert.Equal(t, tt.want, ComponentPath(ctx))
		})
	}
}
