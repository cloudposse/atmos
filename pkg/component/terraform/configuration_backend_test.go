package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
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
		{"tofu retained backend", "backend.tofu", `locals { enabled = true }`, expected, "skip", true},
		{"tofu JSON backend", "backend.tofu.json", `{"locals":{"enabled":true}}`, `{"terraform":{"backend":{"local":{"path":"other"}}}}`, "skip", true},
		{"tofu JSON non backend", "provider.tofu.json", `{"locals":{"enabled":true}}`, `{"locals":{"enabled":false}}`, "skip", false},
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
	if err := os.Symlink(target, filepath.Join(directory, "remote.tf")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	prepared := &schema.PreparedComponentConfiguration{Files: []schema.ComponentConfigurationFile{{Path: "remote.tf", Content: content, IfExists: "skip"}}}
	require.Error(t, writePreparedConfiguration(directory, prepared))
	actual, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, content, actual)
}

// Failures to inspect retained files or the reviewed working directory are classified with a
// static Atmos error and keep the filesystem cause.
func TestRetainedFileFailuresUseStaticErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	retained := schema.ComponentConfigurationFile{Path: "backend.tf", IfExists: "skip", Content: []byte("terraform {}")}
	err := validateRetainedFile(missing, retained)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "backend.tf")

	err = restoreSensitiveContent(missing, []schema.ComponentConfigurationFile{{Path: "secret.auto.tfvars", Sensitive: true}}, "")
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// A sensitive file that its entry removes is absent at apply by design, and restoring it succeeds
// while the digest still binds the reviewed state: a different reviewed digest, a file that
// reappears, or a missing file under any other policy is rejected.
func TestRestoreSensitiveContentAcceptsRemovedFiles(t *testing.T) {
	for _, policy := range []string{"remove", "remove_terragrunt"} {
		t.Run(policy, func(t *testing.T) {
			directory := t.TempDir()
			files := []schema.ComponentConfigurationFile{{Path: "secret.auto.tfvars", Sensitive: true, IfExists: policy}}
			reviewed, err := sensitiveDigest(directory, files)
			require.NoError(t, err)
			require.NoError(t, restoreSensitiveContent(directory, files, reviewed))
			require.Nil(t, files[0].Content)

			require.ErrorIs(t, restoreSensitiveContent(directory, files, "different"), errUtils.ErrPlanVerificationFailed)

			require.NoError(t, os.WriteFile(filepath.Join(directory, "secret.auto.tfvars"), []byte("token=1"), 0o600))
			require.ErrorIs(t, restoreSensitiveContent(directory, files, reviewed), errUtils.ErrPlanVerificationFailed, "a file that appeared after review changes the digest")
		})
	}

	// A file that was already absent when the plan was reviewed, and still is, matches the digest.
	directory := t.TempDir()
	files := []schema.ComponentConfigurationFile{{Path: "secret.auto.tfvars", Sensitive: true, IfExists: "overwrite"}}
	reviewed, err := sensitiveDigest(directory, files)
	require.NoError(t, err)
	require.NoError(t, restoreSensitiveContent(directory, files, reviewed))
	require.Nil(t, files[0].Content)

	// A file that existed at review and has disappeared is rejected as missing.
	path := filepath.Join(directory, "secret.auto.tfvars")
	require.NoError(t, os.WriteFile(path, []byte("token=1"), 0o600))
	reviewed, err = sensitiveDigest(directory, files)
	require.NoError(t, err)
	require.NoError(t, os.Remove(path))
	err = restoreSensitiveContent(directory, files, reviewed)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// The retained-file check excludes backend.tf.json only when its receipt and bytes verify that the
// host handoff removes it; a modified file is still checked and rejected.
func TestRetainedBackendFileExcludedOnlyWhenHandoffRemovesIt(t *testing.T) {
	directory := t.TempDir()
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	info := &schema.ConfigAndStacksInfo{ComponentBackendType: "local", ComponentSettingsSection: map[string]any{"configuration": map[string]any{}}}
	require.NoError(t, backend.GenerateFile(config, info, directory))
	skipped := []schema.ComponentConfigurationFile{{Path: "backend.tf.json", Content: []byte(`{"terraform":{"backend":{"s3":{}}}}`), IfExists: "skip"}}

	require.NoError(t, validateRetainedConfiguration(directory, skipped, true), "the verified host backend is removed by the handoff")
	require.ErrorIs(t, validateRetainedConfiguration(directory, skipped, false), errUtils.ErrPluginInvalidConfig, "without a handoff it stays retained")

	require.NoError(t, os.WriteFile(filepath.Join(directory, "backend.tf.json"), []byte(`{"terraform":{"backend":{"local":{}}}}`), 0o600))
	require.ErrorIs(t, validateRetainedConfiguration(directory, skipped, true), errUtils.ErrPluginInvalidConfig, "a modified file has no matching receipt")
}

// generateHostBackend writes a host-owned backend and its receipt, and returns their contents by path.
func generateHostBackend(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.AutoGenerateBackendFile = true
	info := &schema.ConfigAndStacksInfo{ComponentBackendType: "local", ComponentSettingsSection: map[string]any{"configuration": map[string]any{}}}
	require.NoError(t, backend.GenerateFile(config, info, directory))
	paths := backend.HostBackendPaths()
	require.Len(t, paths, 2)
	contents := make(map[string][]byte, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(path)))
		require.NoError(t, err, path)
		contents[path] = content
	}
	return contents
}

func TestInvalidDestinationPreservesHostBackendAndReceipt(t *testing.T) {
	directory := t.TempDir()
	before := generateHostBackend(t, directory)
	// A directory at an overwrite target makes the replacement batch invalid.
	require.NoError(t, os.Mkdir(filepath.Join(directory, "provider.tf"), 0o700))
	prepared := &schema.PreparedComponentConfiguration{BackendGenerated: true, Files: []schema.ComponentConfigurationFile{
		{Path: "remote.tf", Content: []byte("# generated backend"), IfExists: "overwrite"},
		{Path: "provider.tf", Content: []byte("# generated"), IfExists: "overwrite"},
	}}
	require.Error(t, writePreparedConfiguration(directory, prepared))
	for path, content := range before {
		after, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(path)))
		require.NoError(t, err, path)
		assert.Equal(t, content, after, path)
	}
	assert.NoFileExists(t, filepath.Join(directory, "remote.tf"))
}

func TestHandoffRestoredWhenHandoffFailsAfterRemoval(t *testing.T) {
	directory := t.TempDir()
	before := generateHostBackend(t, directory)
	failure := errUtils.ErrPluginInvalidConfig
	err := configuration.WriteFilesWithHandoff(directory, nil, &configuration.Handoff{
		Paths: backend.HostBackendPaths(),
		Run: func() error {
			require.NoError(t, backend.RemoveHostBackend(directory))
			return failure
		},
	})
	require.ErrorIs(t, err, failure)
	for path, content := range before {
		after, readErr := os.ReadFile(filepath.Join(directory, filepath.FromSlash(path)))
		require.NoError(t, readErr, path)
		assert.Equal(t, content, after, path)
	}
}

func TestValidBatchReplacesHostBackend(t *testing.T) {
	directory := t.TempDir()
	generateHostBackend(t, directory)
	prepared := &schema.PreparedComponentConfiguration{BackendGenerated: true, Files: []schema.ComponentConfigurationFile{
		{Path: "remote.tf", Content: []byte("# generated backend"), IfExists: "overwrite"},
	}}
	require.NoError(t, writePreparedConfiguration(directory, prepared))
	for _, path := range backend.HostBackendPaths() {
		assert.NoFileExists(t, filepath.Join(directory, filepath.FromSlash(path)))
	}
	content, err := os.ReadFile(filepath.Join(directory, "remote.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# generated backend", string(content))
}
