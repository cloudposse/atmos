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

// TestMergeConfig_VersionFilesImportPrecedence exercises version.files through
// REAL import merging (an atmos.yaml importing an atmos.d/ fragment), unlike
// the single-file test above. It pins down a field-test finding: an imported
// fragment can only supply version.files when the main file leaves the key
// unset; it can never override or clear a value the main file already
// declares, including with an explicit `files: []`. This is Atmos's general,
// documented import precedence (see website/docs/cli/configuration/imports.mdx,
// "Merge Order": "Settings in the main atmos.yaml (highest priority)"), not
// something specific to version.files -- this test exists so a future change
// to that general precedence rule doesn't silently break the version.files
// "explicit files: [] suppresses default-path fallback" behavior alongside it.
func TestMergeConfig_VersionFilesImportPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		mainVersion   string
		importVersion string
		assertFiles   func(t *testing.T, files []schema.VersionFileRule)
	}{
		{
			name:        "main omits the key: import's explicit empty list is picked up",
			mainVersion: ``,
			importVersion: `
version:
  files: []
`,
			assertFiles: func(t *testing.T, files []schema.VersionFileRule) {
				t.Helper()
				assert.NotNil(t, files, "the import's files: [] should be picked up when the main file doesn't mention version.files")
				assert.Empty(t, files)
			},
		},
		{
			name: "main sets a real list: import's empty list does NOT override it",
			mainVersion: `
version:
  files:
    - manager: marker
      paths: [Dockerfile]
`,
			importVersion: `
version:
  files: []
`,
			assertFiles: func(t *testing.T, files []schema.VersionFileRule) {
				t.Helper()
				require.Len(t, files, 1, "the main file's own version.files must win over an import's files: []")
				assert.Equal(t, "marker", files[0].Manager)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			importDir := filepath.Join(tempDir, "atmos.d")
			require.NoError(t, os.Mkdir(importDir, 0o755))
			createConfigFile(t, importDir, "version.yaml", tt.importVersion)

			mainContent := `
base_path: "."
import:
  - "./atmos.d/version.yaml"
` + tt.mainVersion
			createConfigFile(t, tempDir, "atmos.yaml", mainContent)

			v := viper.New()
			v.SetConfigType("yaml")
			require.NoError(t, mergeConfig(v, tempDir, CliConfigFileName, true))

			var atmosConfig schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&atmosConfig, atmosDecodeHook()))

			tt.assertFiles(t, atmosConfig.Version.Files)
		})
	}
}
