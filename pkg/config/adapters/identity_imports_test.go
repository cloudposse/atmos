package adapters_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLoadConfig_ImportedIdentitiesTemporarySource uses the real import lifecycle
// with an existing synthetic adapter: its generated file is deleted before the
// final identities and case maps are assembled.
func TestLoadConfig_ImportedIdentitiesTemporarySource(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", dir)
	t.Setenv("AUTH_TEMP_ENDPOINT", "https://example.test")
	config.ResetImportAdapterRegistry()
	t.Cleanup(config.ResetImportAdapterRegistry)
	config.RegisterImportAdapter(&adapters.MockAdapter{MockData: map[string]string{
		"identities": `env:
  Remote_ENV: imported
templates:
  settings:
    env:
      Remote_TEMPLATE: imported
auth:
  identities:
    Remote.Account/Reader:
      kind: aws/permission-set
      via:
        provider: sso
      spec:
        endpoint_url: !env AUTH_TEMP_ENDPOINT
`,
	}})
	path := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`base_path: ./
import: [mock://identities]
auth:
  identities:
    Main.Admin:
      kind: aws/permission-set
    Remote.Account/Reader:
      default: true
`), 0o600))
	cfg, err := config.LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{dir}})
	require.NoError(t, err)
	require.Len(t, cfg.Auth.Identities, 2)
	assert.Contains(t, cfg.Auth.Identities, "main.admin")
	reader := cfg.Auth.Identities["remote.account/reader"]
	assert.Equal(t, "aws/permission-set", reader.Kind)
	assert.True(t, reader.Default)
	require.NotNil(t, reader.Via)
	assert.Equal(t, "sso", reader.Via.Provider)
	assert.Equal(t, "https://example.test", reader.Spec["endpoint_url"])
	assert.Equal(t, "Remote.Account/Reader", cfg.Auth.IdentityCaseMap["remote.account/reader"])
	assert.Equal(t, "imported", cfg.Env["Remote_ENV"])
	assert.Equal(t, "imported", cfg.Templates.Settings.Env["Remote_TEMPLATE"])
	var removedImports []string
	for _, source := range config.LoadedConfigFiles() {
		if strings.Contains(source, "atmos-import-") {
			removedImports = append(removedImports, source)
			assert.NoFileExists(t, source)
		}
	}
	require.Len(t, removedImports, 1, "the test must exercise a deleted temporary import")
}
