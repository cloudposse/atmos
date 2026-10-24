package lockfile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.AtmosConfiguration{BasePathAbsolute: "", BasePathConfigDir: "", CliConfigPath: ""}

func TestProjectPathsHonorResolvedConfigurationBase(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	otherDir := filepath.Join(root, "overlay")
	for _, test := range []struct {
		name   string
		config schema.AtmosConfiguration
		want   string
	}{
		{name: "resolved git root beats nested config source", config: schema.AtmosConfiguration{BasePathAbsolute: root, BasePathConfigDir: configDir, CliConfigPath: configDir + ";" + otherDir + ";"}, want: root},
		{name: "resolved relative base beats raw relative text", config: schema.AtmosConfiguration{BasePath: "./project", BasePathAbsolute: filepath.Join(root, "project"), BasePathConfigDir: configDir}, want: filepath.Join(root, "project")},
		{name: "selected config source replaces joined metadata", config: schema.AtmosConfiguration{BasePathConfigDir: configDir, CliConfigPath: configDir + ";" + otherDir + ";"}, want: configDir},
		{name: "manual explicit base remains authoritative", config: schema.AtmosConfiguration{BasePath: root, BasePathConfigDir: configDir, CliConfigPath: otherDir}, want: root},
		{name: "manual legacy single directory remains supported", config: schema.AtmosConfiguration{CliConfigPath: configDir}, want: configDir},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, filepath.Join(test.want, DefaultFileName), Path(&test.config))
			actual, err := projectBase(&test.config)
			require.NoError(t, err)
			assert.Equal(t, test.want, actual)
			actual, err = lockTargetRoot(&test.config, "vendor")
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(test.want, "vendor"), actual)
		})
	}
}
