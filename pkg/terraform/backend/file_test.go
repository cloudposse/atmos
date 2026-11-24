package backend

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTerraformTestDoesNotGenerateBackend(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "no backend", true: "preserve existing backend"}[existing], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "backend.tf.json")
			const original = `{"terraform":{"backend":{"local":{}}}}`
			if existing {
				require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
			}
			require.NoError(t, GenerateFile(config, &schema.ConfigAndStacksInfo{SubCommand: "test"}, dir))
			if existing {
				content, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, original, string(content))
			} else {
				assert.NoFileExists(t, path)
			}
		})
	}
	for _, command := range []string{"init", "plan", "apply"} {
		t.Run(command+" still requires backend", func(t *testing.T) {
			assert.ErrorIs(t, GenerateFile(config, &schema.ConfigAndStacksInfo{SubCommand: command}, t.TempDir()), errUtils.ErrBackendTypeRequired)
		})
	}
}

func TestGenerateFileConfiguration(t *testing.T) {
	for _, test := range []struct {
		name            string
		enabled, dryRun bool
		backendType     string
		backend         map[string]any
		want            string
	}{
		{name: "local", enabled: true, backendType: "local", backend: map[string]any{"path": "state.tfstate"}, want: `{"terraform":{"backend":{"local":{"path":"state.tfstate"}}}}`},
		{name: "cloud workspace", enabled: true, backendType: "cloud", backend: map[string]any{"workspaces": map[string]any{"name": "{terraform_workspace}"}}, want: `{"terraform":{"cloud":{"workspaces":{"name":"dev"}}}}`},
		{name: "disabled"},
		{name: "dry run", enabled: true, dryRun: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			config := &schema.AtmosConfiguration{}
			config.Components.Terraform.AutoGenerateBackendFile = test.enabled
			info := &schema.ConfigAndStacksInfo{SubCommand: "plan", DryRun: test.dryRun, ComponentBackendType: test.backendType, ComponentBackendSection: test.backend, TerraformWorkspace: "dev"}
			require.NoError(t, GenerateFile(config, info, dir))
			path := filepath.Join(dir, "backend.tf.json")
			if test.want == "" {
				assert.NoFileExists(t, path)
				return
			}
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(content))
		})
	}
}

func TestHostBackendOwnership(t *testing.T) {
	for _, mode := range []string{"unchanged", "modified", "unrecorded", "symlink", "missing", "corrupt receipt"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			config := &schema.AtmosConfiguration{}
			config.Components.Terraform.AutoGenerateBackendFile = true
			info := &schema.ConfigAndStacksInfo{ComponentBackendType: "local", ComponentSettingsSection: map[string]any{"configuration": map[string]any{schema.ConfigurationOwnedKey: true}}}
			require.NoError(t, GenerateFile(config, info, dir))
			path := filepath.Join(dir, hostBackendName)
			switch mode {
			case "modified":
				require.NoError(t, os.WriteFile(path, []byte("user changes"), 0o600))
			case "unrecorded":
				require.NoError(t, os.Remove(filepath.Join(dir, hostBackendReceipt)))
			case "symlink":
				require.NoError(t, os.Rename(path, filepath.Join(dir, "user.tf.json")))
				require.NoError(t, os.Symlink("user.tf.json", path))
			case "missing":
				require.NoError(t, os.Remove(path))
			case "corrupt receipt":
				require.NoError(t, os.WriteFile(filepath.Join(dir, hostBackendReceipt), []byte("invalid"), 0o600))
			}
			err := RemoveHostBackend(dir)
			if mode == "modified" || mode == "symlink" || mode == "corrupt receipt" {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				assert.FileExists(t, path)
			} else {
				require.NoError(t, err)
				if mode == "unrecorded" {
					assert.FileExists(t, path)
				} else {
					assert.NoFileExists(t, path)
				}
			}
		})
	}
}

// The user's own settings.configuration data writes the backend as v1.234.1 did: no ownership
// receipt or other file is added next to the component.
func TestGenerateFileIgnoresUserSettingsConfiguration(t *testing.T) {
	dir := t.TempDir()
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan", ComponentBackendType: "local", ComponentSettingsSection: map[string]any{"configuration": map[string]any{"notes": "internal tooling", "provider": "aws"}}}
	require.NoError(t, GenerateFile(config, info, dir))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, hostBackendName, entries[0].Name())
	assert.NoFileExists(t, filepath.Join(dir, hostBackendReceipt))
}

// A provider-owned backend whose ownership receipt cannot be written fails with the backend
// generation error and keeps the filesystem cause.
func TestGenerateFileWrapsReceiptFailures(t *testing.T) {
	dir := t.TempDir()
	// A file where the receipt directory belongs makes the receipt impossible to create.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".atmos"), nil, 0o600))
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	info := &schema.ConfigAndStacksInfo{
		SubCommand: "plan", ComponentBackendType: "local", ComponentBackendSection: map[string]any{"path": "state.tfstate"},
		ComponentSettingsSection: map[string]any{"configuration": map[string]any{"provider": "terragrunt", schema.ConfigurationOwnedKey: true}},
	}
	err := GenerateFile(config, info, dir)
	require.ErrorIs(t, err, errUtils.ErrBackendFileGeneration)
	require.ErrorContains(t, err, "backend.tf.json")
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr, "the filesystem cause is preserved")
}

// Filesystem failures while relinquishing the host backend are classified with repository
// sentinels and keep their causes; a missing receipt is still not an error.
func TestRemoveHostBackendWrapsFilesystemFailures(t *testing.T) {
	err := RemoveHostBackend(filepath.Join(t.TempDir(), "absent"))
	require.ErrorIs(t, err, errUtils.ErrOpenFile)
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, RemoveHostBackend(t.TempDir()), "no receipt means nothing to remove")

	dir := t.TempDir()
	// A directory where the receipt file belongs cannot be read as a receipt.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".atmos", "host-backend.sha256"), 0o700))
	err = RemoveHostBackend(dir)
	require.ErrorIs(t, err, errUtils.ErrReadFile)
	require.ErrorContains(t, err, "host-backend.sha256")
}

// The generated backend and its receipt are written under the workdir lock that RemoveHostBackend's
// batch holds, so a backend written after a digest check cannot be removed by that handoff.
func TestGenerateFileWaitsForWorkdirLock(t *testing.T) {
	dir := t.TempDir()
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	info := &schema.ConfigAndStacksInfo{ComponentBackendType: "local", ComponentSettingsSection: map[string]any{"configuration": map[string]any{schema.ConfigurationOwnedKey: true}}}
	path := filepath.Join(dir, hostBackendName)
	done := make(chan error, 1)
	require.NoError(t, configuration.WithWorkdirLock(dir, func() error {
		go func() { done <- GenerateFile(config, info, dir) }()
		time.Sleep(100 * time.Millisecond)
		assert.NoFileExists(t, path, "the write waits for the lock holder")
		return nil
	}))
	require.NoError(t, <-done)
	assert.FileExists(t, path)
	assert.FileExists(t, filepath.Join(dir, hostBackendReceipt))
}
