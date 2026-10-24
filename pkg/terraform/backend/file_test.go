package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
