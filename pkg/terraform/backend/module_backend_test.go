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

// ownedInfo is a provider-owned component whose provider reports a local backend it does not generate.
func ownedInfo() *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{
		SubCommand:              "plan",
		ComponentBackendType:    "local",
		ComponentBackendSection: map[string]any{"path": "/state/unit.tfstate"},
		ComponentSettingsSection: map[string]any{
			"configuration": map[string]any{schema.ConfigurationOwnedKey: true},
		},
		ComponentExecution: &schema.ComponentExecution{
			ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}}},
		},
	}
}

// When the module already declares a backend, a second backend file would make Terraform fail with
// "Duplicate backend configuration". The host then passes the unit's backend settings to init as a
// backend configuration file instead, as Terragrunt passes them with -backend-config, and removes the
// backend file it wrote on an earlier run.
func TestGenerateFilePassesTheBackendToInitWhenTheModuleDeclaresOne(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	// OpenTofu reads every form, including `.tofu`, which Terraform ignores.
	config.Components.Terraform.Command = "tofu"
	for name, module := range map[string]string{
		"main.tf":      "terraform {\n  backend \"local\" {}\n}\n",
		"main.tf.json": `{"terraform":{"backend":{"local":{}}}}`,
		"main.tofu":    "terraform {\n  required_version = \">= 1\"\n  backend \"local\" {}\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			info := ownedInfo()
			// A backend file this host wrote before the module declared its own backend.
			require.NoError(t, GenerateFile(config, info, dir))
			require.FileExists(t, filepath.Join(dir, hostBackendName))
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(module), 0o600))
			original := info.ComponentExecution

			require.NoError(t, GenerateFile(config, info, dir))
			require.NoError(t, GenerateFile(config, info, dir), "a repeated generation adds the argument once")

			assert.NoFileExists(t, filepath.Join(dir, hostBackendName))
			content, err := os.ReadFile(filepath.Join(dir, backendConfigFileName))
			require.NoError(t, err)
			assert.Contains(t, string(content), "/state/unit.tfstate")
			require.Len(t, info.ComponentExecution.ExtraArguments, 2)
			want := schema.ComponentExtraArguments{
				Commands: []string{"init"}, Arguments: []string{"-backend-config=" + backendConfigFileName},
			}
			assert.Equal(t, want, info.ComponentExecution.ExtraArguments[1])
			assert.Len(t, original.ExtraArguments, 1, "the prepared execution is not modified in place")
		})
	}
}

// A module without a backend gets the host backend file, as before, and init gets no extra argument.
func TestGenerateFileWritesTheBackendWhenTheModuleDeclaresNone(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	dir := t.TempDir()
	resource := []byte("resource \"terraform_data\" \"x\" {}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), resource, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.tf.json"), []byte(`{"variable":{"backend":{}}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.tf"), []byte("terraform {"), 0o600))
	info := ownedInfo()

	require.NoError(t, GenerateFile(config, info, dir))

	assert.FileExists(t, filepath.Join(dir, hostBackendName))
	assert.NoFileExists(t, filepath.Join(dir, backendConfigFileName))
	assert.Len(t, info.ComponentExecution.ExtraArguments, 1)
}

// A component no provider owns keeps the established behavior even when its module declares a backend.
func TestGenerateFileKeepsUnownedComponentsUnchanged(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	dir := t.TempDir()
	module := []byte("terraform {\n  backend \"local\" {}\n}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), module, 0o600))
	info := ownedInfo()
	info.ComponentSettingsSection = map[string]any{}

	require.NoError(t, GenerateFile(config, info, dir))

	assert.FileExists(t, filepath.Join(dir, hostBackendName))
	assert.NoFileExists(t, filepath.Join(dir, backendConfigFileName))
}

// The unit's settings reach init even when no execution was prepared, and a host backend file that was
// changed since this host wrote it is not removed: the failure is reported instead.
func TestPassBackendToInitEdgeCases(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	module := []byte("terraform {\n  backend \"local\" {}\n}\n")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), module, 0o600))
	info := ownedInfo()
	info.ComponentExecution = nil
	require.NoError(t, GenerateFile(config, info, dir))
	require.NotNil(t, info.ComponentExecution)
	assert.Len(t, info.ComponentExecution.ExtraArguments, 1)

	changed := t.TempDir()
	require.NoError(t, GenerateFile(config, ownedInfo(), changed))
	require.NoError(t, os.WriteFile(filepath.Join(changed, hostBackendName), []byte("user changes"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(changed, "main.tf"), module, 0o600))
	err := GenerateFile(config, ownedInfo(), changed)
	require.ErrorIs(t, err, errUtils.ErrBackendFileGeneration)
	assert.FileExists(t, filepath.Join(changed, hostBackendName))
}

// Files that are not root configuration, or whose `terraform` is not a block, declare no backend.
func TestModuleDeclaresBackendIgnoresWhatIsNotABackendBlock(t *testing.T) {
	assert.False(t, moduleDeclaresBackend(filepath.Join(t.TempDir(), "absent"), false))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "attribute.tf"), []byte("terraform = \"backend\"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("terraform { backend \"s3\" {} }\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested.tf"), 0o700))
	assert.False(t, moduleDeclaresBackend(dir, false))
}

// Each engine is judged by the files it reads: Terraform ignores `.tofu` files, and OpenTofu skips a `.tf`
// or `.tf.json` file whose `.tofu` or `.tofu.json` counterpart exists.
func TestModuleDeclaresBackendFollowsTheEngineFileRules(t *testing.T) {
	backendBlock := []byte("terraform {\n  backend \"local\" {}\n}\n")
	backendJSON := []byte(`{"terraform":{"backend":{"local":{}}}}`)
	noBackend := []byte("terraform {\n  required_version = \">= 1.0\"\n}\n")
	for _, test := range []struct {
		name      string
		files     map[string][]byte
		terraform bool
		tofu      bool
	}{
		{"backend in a .tf file", map[string][]byte{"main.tf": backendBlock}, true, true},
		{"backend in a .tofu file", map[string][]byte{"main.tofu": backendBlock}, false, true},
		{"backend in a .tf file shadowed by .tofu", map[string][]byte{"main.tf": backendBlock, "main.tofu": noBackend}, true, false},
		{"backend in a .tf.json shadowed by .tofu.json", map[string][]byte{"main.tf.json": backendJSON, "main.tofu.json": []byte(`{}`)}, true, false},
		{"backend in another .tf file stays visible", map[string][]byte{"backend.tf": backendBlock, "main.tofu": noBackend}, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range test.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), content, 0o600))
			}
			assert.Equal(t, test.terraform, moduleDeclaresBackend(dir, false), "terraform")
			assert.Equal(t, test.tofu, moduleDeclaresBackend(dir, true), "opentofu")
		})
	}
}

// Without an explicit detector, the configured command's executable name selects OpenTofu.
func TestGenerateFileUsesTheEngineDetector(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	backendBlock := []byte("terraform {\n  backend \"local\" {}\n}\n")
	noBackend := []byte("terraform {}\n")
	module := func(t *testing.T) string {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), backendBlock, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tofu"), noBackend, 0o600))
		return dir
	}

	// OpenTofu reads only main.tofu, which declares no backend, so the host backend file is written.
	tofuDir := module(t)
	require.NoError(t, GenerateFile(config, ownedInfo(), tofuDir, WithOpenTofuDetector(func() bool { return true })))
	assert.FileExists(t, filepath.Join(tofuDir, hostBackendName))

	// Terraform reads main.tf, which declares one, so the settings go to init instead.
	tfDir := module(t)
	require.NoError(t, GenerateFile(config, ownedInfo(), tfDir, WithOpenTofuDetector(func() bool { return false })))
	assert.NoFileExists(t, filepath.Join(tfDir, hostBackendName))
	assert.FileExists(t, filepath.Join(tfDir, backendConfigFileName))

	config.Components.Terraform.Command = filepath.Join("bin", "tofu")
	assert.True(t, commandNamesOpenTofu(config))
	config.Components.Terraform.Command = "terraform"
	assert.False(t, commandNamesOpenTofu(config))
	assert.False(t, commandNamesOpenTofu(nil))
}
