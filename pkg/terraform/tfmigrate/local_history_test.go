package tfmigrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTfmigrateConfig(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func TestEnsureLocalHistoryDir_CreatesRelativeDir(t *testing.T) {
	dir := t.TempDir()
	writeTfmigrateConfig(t, dir, defaultConfigFile, `
tfmigrate {
  migration_dir = "./migrations"
  history {
    storage "local" {
      path = "state/tfmigrate-history/service-history.json"
    }
  }
}
`)

	require.NoError(t, EnsureLocalHistoryDir(dir, "", nil))

	info, err := os.Stat(filepath.Join(dir, "state", "tfmigrate-history"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureLocalHistoryDir_EvaluatesEnvReferences(t *testing.T) {
	dir := t.TempDir()
	writeTfmigrateConfig(t, dir, defaultConfigFile, `
tfmigrate {
  history {
    storage "local" {
      path = "${env.ATMOS_TFMIGRATE_HISTORY_KEY}"
    }
  }
}
`)

	env := []string{"ATMOS_TFMIGRATE_HISTORY_KEY=tfmigrate/test/service/default/history.json"}
	require.NoError(t, EnsureLocalHistoryDir(dir, "", env))

	info, err := os.Stat(filepath.Join(dir, "tfmigrate", "test", "service", "default"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureLocalHistoryDir_ExplicitConfigPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "migrations"), 0o755))
	writeTfmigrateConfig(t, dir, filepath.Join("migrations", "custom.hcl"), `
tfmigrate {
  history {
    storage "local" {
      path = "history/service.json"
    }
  }
}
`)

	require.NoError(t, EnsureLocalHistoryDir(dir, filepath.Join("migrations", "custom.hcl"), nil))

	info, err := os.Stat(filepath.Join(dir, "history"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureLocalHistoryDir_NoopWithoutLocalStorage(t *testing.T) {
	dir := t.TempDir()
	writeTfmigrateConfig(t, dir, defaultConfigFile, `
tfmigrate {
  history {
    storage "s3" {
      bucket = "tfstate-bucket"
      key    = "tfmigrate/history.json"
    }
  }
}
`)

	require.NoError(t, EnsureLocalHistoryDir(dir, "", nil))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, defaultConfigFile, entries[0].Name())
}

func TestEnsureLocalHistoryDir_NoopWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, EnsureLocalHistoryDir(dir, "", nil))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestEnsureLocalHistoryDir_NoopOnUnparsableConfig(t *testing.T) {
	dir := t.TempDir()
	writeTfmigrateConfig(t, dir, defaultConfigFile, `tfmigrate { history { storage "local" {`)

	require.NoError(t, EnsureLocalHistoryDir(dir, "", nil))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, defaultConfigFile, entries[0].Name())
}

func TestEnsureLocalHistoryDir_NoopOnUnresolvableEnvReference(t *testing.T) {
	dir := t.TempDir()
	writeTfmigrateConfig(t, dir, defaultConfigFile, `
tfmigrate {
  history {
    storage "local" {
      path = "${env._ATMOS_TFMIGRATE_TEST_UNSET_VARIABLE}/history.json"
    }
  }
}
`)

	require.NoError(t, EnsureLocalHistoryDir(dir, "", nil))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, defaultConfigFile, entries[0].Name())
}

func TestAppendPlanVarFile(t *testing.T) {
	env := AppendPlanVarFile([]string{"A=B"}, "test-service.terraform.tfvars.json")
	assert.Contains(t, env, "TF_CLI_ARGS_plan=-var-file=test-service.terraform.tfvars.json")
	assert.Contains(t, env, "A=B")
}

func TestAppendPlanVarFile_EmptyVarfileIsNoop(t *testing.T) {
	env := AppendPlanVarFile([]string{"A=B"}, "")
	assert.Equal(t, []string{"A=B"}, env)
}

func TestAppendPlanVarFile_ExtendsExistingEntry(t *testing.T) {
	env := AppendPlanVarFile([]string{"TF_CLI_ARGS_plan=-lock=false"}, "vars.tfvars.json")
	assert.Equal(t, []string{"TF_CLI_ARGS_plan=-lock=false -var-file=vars.tfvars.json"}, env)
}

func TestAppendPlanVarFile_ExtendsProcessEnvironment(t *testing.T) {
	t.Setenv(EnvTfCliArgsPlan, "-lock=false")

	env := AppendPlanVarFile(nil, "vars.tfvars.json")
	assert.Equal(t, []string{"TF_CLI_ARGS_plan=-lock=false -var-file=vars.tfvars.json"}, env)
}

func TestEnsureResolved_AcceptsResolvedPath(t *testing.T) {
	require.NoError(t, EnsureResolved(filepath.Join(t.TempDir(), "tools", Command)))
}

func TestEnsureResolved_ErrorsWhenUnresolved(t *testing.T) {
	// Force PATH lookup to fail so the bare command name cannot resolve.
	t.Setenv("PATH", t.TempDir())

	err := EnsureResolved(Command)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command not found")
}
