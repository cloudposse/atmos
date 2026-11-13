package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestPreparedConfigurationRetainedBackend(t *testing.T) {
	const expected = `terraform {
 backend "local" {
  path = "expected.tfstate"
 }
}`
	for _, test := range []struct {
		name, path, generated, retained, policy string
		blocked                                 bool
	}{
		{"missing backend", "remote.tf", expected, `locals { enabled = true }`, "skip", true},
		{"different identity", "remote.tf", expected, `terraform {
 backend "local" {
  path = "other.tfstate"
 }
}`, "skip", true},
		{"unexpected backend", "provider.tf", `locals { enabled = true }`, expected, "skip", true},
		{"unexpected cloud", "provider.tf", `locals { enabled = true }`, "terraform {\n cloud {}\n}", "skip", true},
		{"json backend", "remote.tf.json", `{"terraform":{"backend":{"local":{"path":"expected"}}}}`, `{"terraform":{"backend":{"local":{"path":"other"}}}}`, "skip", true},
		{"invalid retained", "remote.tf", expected, `terraform {`, "skip", true},
		{"identical backend", "remote.tf", expected, expected, "skip", false},
		{"non backend", "provider.tf", `locals { enabled = true }`, `locals { enabled = false }`, "skip", false},
		{"auxiliary", "data.json", `{"value":true}`, `{"value":false}`, "skip", false},
		{"overwrite", "remote.tf", expected, `locals { enabled = true }`, "overwrite", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, test.path)
			require.NoError(t, os.WriteFile(path, []byte(test.retained), 0o600))
			prepared := &schema.PreparedComponentConfiguration{Files: []schema.ComponentConfigurationFile{{Path: test.path, Content: []byte(test.generated), IfExists: test.policy}}}
			err := writePreparedConfiguration(directory, prepared)
			if test.blocked {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			} else {
				require.NoError(t, err)
			}
			content, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			want := test.retained
			if test.policy == "overwrite" {
				want = test.generated
			}
			require.Equal(t, want, string(content))
		})
	}
}

// Terragrunt writes `remote_state.encryption` only into the file `remote_state.generate` produces,
// together with the backend block. A different file kept by if_exists = "skip" would drop the
// encryption and let OpenTofu write plain state, so it is refused before anything is written.
func TestPreparedConfigurationRetainedBackendKeepsEncryption(t *testing.T) {
	const generated = `terraform {
 backend "local" {
  path = "expected.tfstate"
 }
 encryption {
  key_provider "pbkdf2" "default" {
   passphrase = "correct-horse-battery-staple"
  }
 }
}`
	for _, test := range []struct {
		name, retained string
		blocked        bool
	}{
		{"same backend without encryption", "terraform {\n backend \"local\" {\n  path = \"expected.tfstate\"\n }\n}", true},
		{"no backend and no encryption", "locals { enabled = true }", true},
		{"identical encrypted backend", generated, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "backend.tf")
			require.NoError(t, os.WriteFile(path, []byte(test.retained), 0o600))
			prepared := &schema.PreparedComponentConfiguration{BackendGenerated: true, Files: []schema.ComponentConfigurationFile{{Path: "backend.tf", Content: []byte(generated), IfExists: "skip"}}}
			err := writePreparedConfiguration(directory, prepared)
			if !test.blocked {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			assert.NotContains(t, err.Error(), "correct-horse-battery-staple", "the error never shows the generated secrets")
			content, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, test.retained, string(content), "the retained file is left alone")
		})
	}
}

func TestRetainedBackendFailurePreservesHostBackend(t *testing.T) {
	directory := t.TempDir()
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	info := &schema.ConfigAndStacksInfo{ComponentBackendType: "local", ComponentSettingsSection: map[string]any{"configuration": map[string]any{}}}
	require.NoError(t, backend.GenerateFile(config, info, directory))
	hostPath := filepath.Join(directory, "backend.tf.json")
	before, err := os.ReadFile(hostPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "remote.tf"), []byte("# user file"), 0o600))
	prepared := &schema.PreparedComponentConfiguration{BackendGenerated: true, Files: []schema.ComponentConfigurationFile{{Path: "remote.tf", Content: []byte("terraform {\n backend \"local\" {}\n}"), IfExists: "skip"}}}
	require.ErrorIs(t, writePreparedConfiguration(directory, prepared), errUtils.ErrPluginInvalidConfig)
	after, err := os.ReadFile(hostPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestPreparedConfigurationSkipNewBackend(t *testing.T) {
	directory := t.TempDir()
	content := []byte("terraform {\n backend \"local\" {}\n}")
	prepared := &schema.PreparedComponentConfiguration{BackendGenerated: true, Files: []schema.ComponentConfigurationFile{{Path: "remote.tf", Content: content, IfExists: "skip"}}}
	require.NoError(t, writePreparedConfiguration(directory, prepared))
	actual, err := os.ReadFile(filepath.Join(directory, "remote.tf"))
	require.NoError(t, err)
	require.Equal(t, content, actual)
	require.NoError(t, writePreparedConfiguration(directory, prepared))
}

func TestPreparedConfigurationSkipSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.tf")
	content := []byte("# user-owned")
	require.NoError(t, os.WriteFile(target, content, 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(directory, "remote.tf")))
	prepared := &schema.PreparedComponentConfiguration{Files: []schema.ComponentConfigurationFile{{Path: "remote.tf", Content: content, IfExists: "skip"}}}
	require.Error(t, writePreparedConfiguration(directory, prepared))
	actual, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, content, actual)
}
