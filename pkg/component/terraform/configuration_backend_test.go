package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/backend"
)

func TestPreparedBackendTakeover(t *testing.T) {
	for _, name := range []string{"remote.tf", "backend.tf.json"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			config := &schema.AtmosConfiguration{}
			config.Components.Terraform.AutoGenerateBackendFile = true
			info := &schema.ConfigAndStacksInfo{ComponentBackendType: "local", ComponentSettingsSection: map[string]any{"configuration": map[string]any{}}}
			require.NoError(t, backend.GenerateFile(config, info, directory))
			userFile := filepath.Join(directory, "user.tf")
			require.NoError(t, os.WriteFile(userFile, []byte("# user-owned"), 0o600))
			prepared := &schema.PreparedComponentConfiguration{BackendGenerated: true, Files: []schema.ComponentConfigurationFile{{Path: name, Content: []byte("# generated backend"), IfExists: "error"}}}
			require.NoError(t, writePreparedConfiguration(directory, prepared))
			content, err := os.ReadFile(filepath.Join(directory, name))
			require.NoError(t, err)
			assert.Equal(t, "# generated backend", string(content))
			if name != "backend.tf.json" {
				assert.NoFileExists(t, filepath.Join(directory, "backend.tf.json"))
			}
			content, err = os.ReadFile(userFile)
			require.NoError(t, err)
			assert.Equal(t, "# user-owned", string(content))
			// Subsequent preparation uses provider ownership, with no stale host receipt.
			prepared.Files[0].IfExists = "overwrite"
			require.NoError(t, writePreparedConfiguration(directory, prepared))
		})
	}
}
