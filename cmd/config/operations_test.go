package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func TestConfigCommands_EditCurrentDirectoryConfig(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("settings:\n  enabled: false\n  stale: yes\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
		valueType = atmosyaml.TypeString
	})
	require.NoError(t, os.Chdir(dir))

	valueType = atmosyaml.TypeBool
	require.NoError(t, configSetCmd.RunE(configSetCmd, []string{"settings.enabled", "true"}))
	got, err := atmosyaml.GetFile(file, "settings.enabled")
	require.NoError(t, err)
	assert.Equal(t, "true", got)

	require.NoError(t, configDeleteCmd.RunE(configDeleteCmd, []string{"settings.stale"}))
	_, err = atmosyaml.GetFile(file, "settings.stale")
	require.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

func TestConfigFormatCommand_FormatsCurrentDirectoryConfig(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("settings: {enabled: true}\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, configFormatCmd.RunE(configFormatCmd, nil))
	got, err := atmosyaml.GetFile(file, "settings.enabled")
	require.NoError(t, err)
	assert.Equal(t, "true", got)
	formatted, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.NotEmpty(t, formatted)
}

func TestResolveConfigFile_OverrideFlag(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "custom.yaml")
	require.NoError(t, os.WriteFile(file, []byte("settings:\n  enabled: true\n"), 0o644))

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("config", []string{file}, "")

	got, err := resolveConfigFile(cmd)
	require.NoError(t, err)
	assert.Equal(t, file, got)
}

func TestResolveConfigFile_Error(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("config", []string{filepath.Join(t.TempDir(), "missing.yaml")}, "")

	_, err := resolveConfigFile(cmd)
	require.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

func TestCommandProvider(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	SetAtmosConfig(cfg)
	t.Cleanup(func() {
		SetAtmosConfig(nil)
	})
	assert.Same(t, cfg, atmosConfigPtr)

	provider := &CommandProvider{}
	assert.Same(t, configCmd, provider.GetCommand())
	assert.Equal(t, "config", provider.GetName())
	assert.Equal(t, "Configuration Management", provider.GetGroup())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.Nil(t, provider.GetAliases())
	assert.False(t, provider.IsExperimental())
}
