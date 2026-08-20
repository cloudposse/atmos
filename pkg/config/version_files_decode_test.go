package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLoadConfigFromCLIArgs_VersionFilesNilVsExplicitEmpty establishes, empirically, that the
// Viper + mapstructure config-loading pipeline preserves the distinction between an OMITTED
// `version.files` key and an EXPLICITLY EMPTY `version.files: []`. This underpins the fix to
// pkg/version/managers.fileRules(), which switched from `len(Version.Files) > 0` (unable to tell
// the two cases apart) to `Version.Files != nil` (omitted -> nil -> fall back to manager
// defaults; explicit empty -> non-nil, zero-length -> manage zero files). If a future
// Viper/mergo/mapstructure change collapsed this distinction, this test must fail here rather
// than silently resurrecting the "can't configure zero managed files" bug.
func TestLoadConfigFromCLIArgs_VersionFilesNilVsExplicitEmpty(t *testing.T) {
	tests := []struct {
		name        string
		versionYAML string
		assertFiles func(t *testing.T, files []schema.VersionFileRule)
	}{
		{
			name:        "omitted key stays nil",
			versionYAML: ``,
			assertFiles: func(t *testing.T, files []schema.VersionFileRule) {
				t.Helper()
				assert.Nil(t, files, "version.files must stay nil when the key is omitted, so "+
					"fileRules() can fall back to manager defaults")
			},
		},
		{
			name: "explicit empty list decodes to a non-nil, zero-length slice",
			versionYAML: `
version:
  files: []
`,
			assertFiles: func(t *testing.T, files []schema.VersionFileRule) {
				t.Helper()
				assert.NotNil(t, files, "explicit `files: []` must decode to a non-nil slice, so "+
					"fileRules() can distinguish it from an omitted key")
				assert.Empty(t, files)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			mainFile := filepath.Join(tmpDir, "atmos.yaml")

			require.NoError(t, os.WriteFile(mainFile, []byte(`
base_path: "."
components:
  terraform:
    base_path: "components/terraform"
`+tt.versionYAML), 0o644))

			v := viper.New()
			v.SetConfigType("yaml")

			configAndStacksInfo := &schema.ConfigAndStacksInfo{
				AtmosConfigFilesFromArg: []string{mainFile},
			}

			var atmosConfig schema.AtmosConfiguration
			err := loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
			require.NoError(t, err)

			tt.assertFiles(t, atmosConfig.Version.Files)
		})
	}
}
