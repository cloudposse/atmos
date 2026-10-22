package initcmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	projectconfig "github.com/cloudposse/atmos/pkg/project/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFieldTestAtmosTemplateLoads(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, executeInit(context.Background(), &initOptions{templateName: "atmos", targetDir: dir}))
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_BASE_PATH", dir)
	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err, "the generated CLI configuration must load without manual edits")
}

func TestFieldTestInitUpdateFromRecordedTemplate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, executeInit(context.Background(), &initOptions{templateName: "basic", targetDir: dir, templateVars: map[string]any{"project_name": "saved-project"}}))
	for _, args := range [][]string{{"init"}, {"config", "commit.gpgsign", "false"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}, {"add", "."}, {"commit", "-m", "initial"}} {
		require.NoError(t, runGitCommand(t, dir, args...))
	}
	readme := filepath.Join(dir, "README.md")
	content, err := os.ReadFile(readme)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(readme, append(content, []byte("\nUser customization.\n")...), 0o600))
	t.Chdir(dir)
	require.NoError(t, executeInit(context.Background(), &initOptions{update: true}))
	record, err := projectconfig.LoadProjectRecord(dir)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "basic", record.Metadata.Name)
	assert.Equal(t, "saved-project", record.Spec.Values["project_name"])
	updated, err := os.ReadFile(readme)
	require.NoError(t, err)
	assert.Contains(t, string(updated), "User customization.")
}
