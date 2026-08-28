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

func TestStripMigrationDirPrefix(t *testing.T) {
	componentDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(componentDir, "migrations"), 0o755))

	tests := []struct {
		name      string
		migration string
		want      string
	}{
		{name: "strips matching migrations/ prefix", migration: "migrations/foo.hcl", want: "foo.hcl"},
		{name: "strips matching ./migrations/ prefix", migration: "./migrations/foo.hcl", want: "foo.hcl"},
		{name: "bare filename unchanged", migration: "foo.hcl", want: "foo.hcl"},
		{name: "unrelated prefix unchanged", migration: "other/foo.hcl", want: "other/foo.hcl"},
		{name: "empty stays empty", migration: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, StripMigrationDirPrefix(tt.migration, componentDir))
		})
	}
}

func TestStripMigrationDirPrefix_NoMigrationsDirLeavesPathUnchanged(t *testing.T) {
	// migrationDirFor falls back to "." (the component root) when there's no
	// migrations/ subdirectory - nothing to strip in that case.
	componentDir := t.TempDir()
	assert.Equal(t, "migrations/foo.hcl", StripMigrationDirPrefix("migrations/foo.hcl", componentDir))
}

func TestHasMigrationsDir(t *testing.T) {
	t.Run("true when migrations/ subdirectory exists", func(t *testing.T) {
		componentDir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(componentDir, "migrations"), 0o755))
		assert.True(t, HasMigrationsDir(componentDir))
	})

	t.Run("false when there is no migrations/ subdirectory", func(t *testing.T) {
		assert.False(t, HasMigrationsDir(t.TempDir()))
	})

	t.Run("false when migrations is a file, not a directory", func(t *testing.T) {
		componentDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(componentDir, "migrations"), []byte("not a dir"), 0o644))
		assert.False(t, HasMigrationsDir(componentDir))
	})
}

func TestNoMigrationsToRun(t *testing.T) {
	withMigrationsDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(withMigrationsDir, "migrations"), 0o755))
	withoutMigrationsDir := t.TempDir()

	tests := []struct {
		name         string
		migration    string
		componentDir string
		want         bool
	}{
		{name: "history mode, no migrations dir: nothing to run", migration: "", componentDir: withoutMigrationsDir, want: true},
		{name: "history mode, migrations dir exists: has something to run", migration: "", componentDir: withMigrationsDir, want: false},
		{name: "explicit --migration, no migrations dir: still has something to run", migration: "foo.hcl", componentDir: withoutMigrationsDir, want: false},
		{name: "explicit --migration, migrations dir exists: has something to run", migration: "migrations/foo.hcl", componentDir: withMigrationsDir, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NoMigrationsToRun(tt.migration, tt.componentDir))
		})
	}
}
