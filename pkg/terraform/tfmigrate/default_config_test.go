package tfmigrate

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func defaultConfigHistory() HistoryValues {
	return HistoryNames("test", "service", "")
}

// normalizeHCL collapses hclwrite's column alignment so assertions can use
// single-space `attr = value` forms.
func normalizeHCL(hcl string) string {
	return regexp.MustCompile(`[ \t]+`).ReplaceAllString(hcl, " ")
}

func TestDefaultConfigHCL_S3Backend(t *testing.T) {
	hcl := normalizeHCL(DefaultConfigHCL(&DefaultConfigInput{
		ComponentDir: t.TempDir(),
		BackendType:  "s3",
		Backend: map[string]any{
			"bucket": "tfstate-bucket",
			"region": "us-east-1",
			"assume_role": map[string]any{
				"role_arn": "arn:aws:iam::123456789012:role/tfstate",
			},
		},
		History: defaultConfigHistory(),
	}))

	assert.Contains(t, hcl, `storage "s3"`)
	assert.Contains(t, hcl, `bucket = "tfstate-bucket"`)
	assert.Contains(t, hcl, `key = "tfmigrate/test/service/default/history.json"`)
	assert.Contains(t, hcl, `region = "us-east-1"`)
	assert.Contains(t, hcl, `role_arn = "arn:aws:iam::123456789012:role/tfstate"`)
	assert.NotContains(t, hcl, "endpoint")
	assert.Contains(t, hcl, `migration_dir = "."`)
}

func TestDefaultConfigHCL_S3BackendWithNestedEndpoint(t *testing.T) {
	hcl := normalizeHCL(DefaultConfigHCL(&DefaultConfigInput{
		ComponentDir: t.TempDir(),
		BackendType:  "s3",
		Backend: map[string]any{
			"bucket": "tfstate-bucket",
			"region": "us-east-1",
			"endpoints": map[string]any{
				"s3": "http://localhost:4566",
			},
		},
		History: defaultConfigHistory(),
	}))

	assert.Contains(t, hcl, `endpoint = "http://localhost:4566"`)
	assert.Contains(t, hcl, "force_path_style = true")
	assert.Contains(t, hcl, "skip_credentials_validation = true")
	assert.Contains(t, hcl, "skip_metadata_api_check = true")
}

func TestDefaultConfigHCL_GCSBackend(t *testing.T) {
	hcl := normalizeHCL(DefaultConfigHCL(&DefaultConfigInput{
		ComponentDir: t.TempDir(),
		BackendType:  "gcs",
		Backend:      map[string]any{"bucket": "tfstate-bucket"},
		History:      defaultConfigHistory(),
	}))

	assert.Contains(t, hcl, `storage "gcs"`)
	assert.Contains(t, hcl, `bucket = "tfstate-bucket"`)
	assert.Contains(t, hcl, `name = "tfmigrate/test/service/default/history.json"`)
}

func TestDefaultConfigHCL_LocalBackendStoresHistoryBesideState(t *testing.T) {
	hcl := normalizeHCL(DefaultConfigHCL(&DefaultConfigInput{
		ComponentDir: t.TempDir(),
		BackendType:  "local",
		Backend:      map[string]any{"path": "../../../state/service.tfstate"},
		History:      defaultConfigHistory(),
	}))

	assert.Contains(t, hcl, `storage "local"`)
	assert.Contains(t, hcl, `path = "../../../state/tfmigrate/test/service/default/history.json"`)
}

func TestDefaultConfigHCL_NoBackendFallsBackToWorkdirHistory(t *testing.T) {
	hcl := normalizeHCL(DefaultConfigHCL(&DefaultConfigInput{
		ComponentDir: t.TempDir(),
		History:      defaultConfigHistory(),
	}))

	assert.Contains(t, hcl, `storage "local"`)
	assert.Contains(t, hcl, `path = "tfmigrate/test/service/default/history.json"`)
}

func TestDefaultConfigHCL_DetectsMigrationsDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "migrations"), 0o755))

	hcl := normalizeHCL(DefaultConfigHCL(&DefaultConfigInput{ComponentDir: dir, History: defaultConfigHistory()}))

	assert.Contains(t, hcl, `migration_dir = "./migrations"`)
}

func TestEnsureDefaultConfig_GeneratesAndCleansUp(t *testing.T) {
	dir := t.TempDir()

	path, cleanup, err := EnsureDefaultConfig(&DefaultConfigInput{
		ComponentDir: dir,
		BackendType:  "s3",
		Backend:      map[string]any{"bucket": "tfstate-bucket"},
		History:      defaultConfigHistory(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), `storage "s3"`)

	cleanup()
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestEnsureDefaultConfig_RespectsExistingConfigFile(t *testing.T) {
	dir := t.TempDir()
	writeTfmigrateConfig(t, dir, defaultConfigFile, "tfmigrate {}\n")

	path, cleanup, err := EnsureDefaultConfig(&DefaultConfigInput{ComponentDir: dir, History: defaultConfigHistory()})
	require.NoError(t, err)
	assert.Empty(t, path)
	assert.Nil(t, cleanup)
}

func TestEnsureDefaultConfig_RespectsTfmigrateConfigEnvVar(t *testing.T) {
	t.Setenv(ConfigEnvVar, filepath.Join(t.TempDir(), "custom.hcl"))

	path, cleanup, err := EnsureDefaultConfig(&DefaultConfigInput{ComponentDir: t.TempDir(), History: defaultConfigHistory()})
	require.NoError(t, err)
	assert.Empty(t, path)
	assert.Nil(t, cleanup)
}

func TestEnsureDefaultConfig_PropagatesCreateTempError(t *testing.T) {
	// os.CreateTemp("", ...) resolves the target directory via os.TempDir(),
	// which reads TMPDIR (and TMP/TEMP on Windows). Pointing all three at a
	// nonexistent directory makes CreateTemp fail deterministically, exercising
	// the error path without any OS-level fault injection.
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("TMPDIR", missing)
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)

	_, _, err := EnsureDefaultConfig(&DefaultConfigInput{ComponentDir: t.TempDir(), History: defaultConfigHistory()})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCreateFile)
}

func TestDefaultConfigHCL_GCSBackendNestedBlock(t *testing.T) {
	// Atmos component backend sections nest provider-specific config under the
	// backend type key, e.g. `backend: { gcs: { bucket: ... } }`, mirroring how
	// BackendHistoryValues handles the same nested shape.
	hcl := normalizeHCL(DefaultConfigHCL(&DefaultConfigInput{
		ComponentDir: t.TempDir(),
		BackendType:  "gcs",
		Backend: map[string]any{
			"gcs": map[string]any{"bucket": "nested-bucket"},
		},
		History: defaultConfigHistory(),
	}))

	assert.Contains(t, hcl, `storage "gcs"`)
	assert.Contains(t, hcl, `bucket = "nested-bucket"`)
}
