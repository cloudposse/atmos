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

// TestConfigSetCommand_CreatedVsUpdated exercises both branches of the
// created/updated distinction: RunE must succeed either way, so this only
// asserts on the resulting file state (message wording is covered manually).
func TestConfigSetCommand_CreatedVsUpdated(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("settings:\n  enabled: false\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
		valueType = atmosyaml.TypeString
	})
	require.NoError(t, os.Chdir(dir))

	// settings.region does not exist yet -- created branch.
	require.NoError(t, configSetCmd.RunE(configSetCmd, []string{"settings.region", "us-east-1"}))
	got, err := atmosyaml.GetFile(file, "settings.region")
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", got)

	// settings.region now exists -- updated branch.
	require.NoError(t, configSetCmd.RunE(configSetCmd, []string{"settings.region", "us-west-2"}))
	got, err = atmosyaml.GetFile(file, "settings.region")
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", got)
}

// TestConfigDeleteCommand_NothingToDelete exercises the delete/no-op
// distinction: deleting an absent path must still succeed (not error), and
// must not disturb the rest of the file.
func TestConfigDeleteCommand_NothingToDelete(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("settings:\n  enabled: false\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, configDeleteCmd.RunE(configDeleteCmd, []string{"settings.does_not_exist"}))
	got, err := atmosyaml.GetFile(file, "settings.enabled")
	require.NoError(t, err)
	assert.Equal(t, "false", got, "an absent-path delete must not touch the rest of the file")
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

// TestResolveConfigFile_MultipleOverrides_TargetsOnlyFirst covers a real
// inconsistency: --config normally merges every listed file (all other Atmos
// commands), but edit commands can only ever write to one physical file, so
// only the first entry is targeted -- the rest are silently unused. This
// asserts the functional behavior (only the first file is resolved/touched);
// the accompanying warning's wording is covered by live verification, per
// this file's established convention of not unit-testing ui.* message text
// (see TestConfigSetCommand_CreatedVsUpdated).
func TestResolveConfigFile_MultipleOverrides_TargetsOnlyFirst(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.yaml")
	second := filepath.Join(dir, "b.yaml")
	require.NoError(t, os.WriteFile(first, []byte("settings:\n  enabled: true\n"), 0o644))
	require.NoError(t, os.WriteFile(second, []byte("settings:\n  enabled: false\n"), 0o644))

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("config", []string{first, second}, "")

	got, err := resolveConfigFile(cmd)
	require.NoError(t, err)
	assert.Equal(t, first, got)

	// The second file must remain untouched by any subsequent edit.
	require.NoError(t, atmosyaml.SetFile(got, "settings.enabled", "changed"))
	untouched, err := os.ReadFile(second)
	require.NoError(t, err)
	assert.Contains(t, string(untouched), "enabled: false")
}

func TestResolveConfigFile_Error(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("config", []string{filepath.Join(t.TempDir(), "missing.yaml")}, "")

	_, err := resolveConfigFile(cmd)
	require.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

func TestConfigGetCommand_MissingValue(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("settings:\n  enabled: true\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))

	err = configGetCmd.RunE(configGetCmd, []string{"settings.does_not_exist"})
	require.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

func TestConfigSetCommand_TypeVariants(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte(
		"settings:\n  region: us-east-1\n  count: 1\n  enabled: false\n  raw: old\n",
	), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
		valueType = atmosyaml.TypeAuto
	})
	require.NoError(t, os.Chdir(dir))

	tests := []struct {
		name      string
		path      string
		value     string
		valueType string
		assert    func(t *testing.T)
	}{
		{
			name:      "bool",
			path:      "settings.enabled",
			value:     " TRUE ",
			valueType: atmosyaml.TypeBool,
			assert: func(t *testing.T) {
				t.Helper()
				got, getErr := atmosyaml.GetFile(file, "settings.enabled")
				require.NoError(t, getErr)
				assert.Equal(t, "true", got)
			},
		},
		{
			name:      "int",
			path:      "settings.count",
			value:     "42",
			valueType: atmosyaml.TypeInt,
			assert: func(t *testing.T) {
				t.Helper()
				got, getErr := atmosyaml.GetFile(file, "settings.count")
				require.NoError(t, getErr)
				assert.Equal(t, "42", got)
			},
		},
		{
			name:      "float",
			path:      "settings.count",
			value:     "3.14",
			valueType: atmosyaml.TypeFloat,
			assert: func(t *testing.T) {
				t.Helper()
				got, getErr := atmosyaml.GetFile(file, "settings.count")
				require.NoError(t, getErr)
				assert.Equal(t, "3.14", got)
			},
		},
		{
			name:      "null",
			path:      "settings.region",
			value:     "ignored",
			valueType: atmosyaml.TypeNull,
			assert: func(t *testing.T) {
				t.Helper()
				// A null value is indistinguishable from "not present" for Get,
				// so it surfaces as ErrYAMLPathNotFound (see pkg/yaml.Get).
				_, getErr := atmosyaml.GetFile(file, "settings.region")
				require.ErrorIs(t, getErr, atmosyaml.ErrYAMLPathNotFound)
			},
		},
		{
			name:      "yaml",
			path:      "settings.raw",
			value:     "[1, 2, 3]",
			valueType: atmosyaml.TypeYAML,
			assert: func(t *testing.T) {
				t.Helper()
				content, readErr := os.ReadFile(file)
				require.NoError(t, readErr)
				assert.Contains(t, string(content), "- 1")
				assert.Contains(t, string(content), "- 2")
				assert.Contains(t, string(content), "- 3")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valueType = tt.valueType
			require.NoError(t, configSetCmd.RunE(configSetCmd, []string{tt.path, tt.value}))
			tt.assert(t)
		})
	}
}

// TestConfigSetCommand_AutoInfersFromExistingValue covers --type=auto (the
// default) on a path the Atmos config schema doesn't model (settings.* here
// is a free-form test fixture, not a real modeled field): when the path
// already has a typed value, auto must infer that type instead of falling
// back to string.
func TestConfigSetCommand_AutoInfersFromExistingValue(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte(
		"settings:\n  replicas: 1\n  enabled: false\n",
	), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
		valueType = atmosyaml.TypeAuto
	})
	require.NoError(t, os.Chdir(dir))

	valueType = atmosyaml.TypeAuto
	require.NoError(t, configSetCmd.RunE(configSetCmd, []string{"settings.replicas", "5"}))
	require.NoError(t, configSetCmd.RunE(configSetCmd, []string{"settings.enabled", "true"}))

	content, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(content), "replicas: 5")
	assert.NotContains(t, string(content), `replicas: "5"`)
	assert.Contains(t, string(content), "enabled: true")
	assert.NotContains(t, string(content), `enabled: "true"`)
}

// TestConfigSetCommand_AutoFallsBackToStringForNewKey covers --type=auto when
// the path is neither schema-modeled nor already present in the file -- there
// is nothing to infer from, so it must fall back to string.
func TestConfigSetCommand_AutoFallsBackToStringForNewKey(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("base_path: \"./\"\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
		valueType = atmosyaml.TypeAuto
	})
	require.NoError(t, os.Chdir(dir))

	valueType = atmosyaml.TypeAuto
	require.NoError(t, configSetCmd.RunE(configSetCmd, []string{"settings.replicas", "5"}))

	got, err := atmosyaml.GetFile(file, "settings.replicas")
	require.NoError(t, err)
	assert.Equal(t, "5", got)

	content, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(content), `replicas: "5"`)
}

func TestConfigSetCommand_InvalidType(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("settings:\n  count: 1\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
		valueType = atmosyaml.TypeString
	})
	require.NoError(t, os.Chdir(dir))

	valueType = atmosyaml.TypeInt
	err = configSetCmd.RunE(configSetCmd, []string{"settings.count", "not-an-int"})
	require.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
}

func TestConfigDeleteCommand_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("settings:\n  enabled: true\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))

	err = configDeleteCmd.RunE(configDeleteCmd, []string{"a..b"})
	require.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
}

func TestConfigFormatCommand_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	// Unbalanced flow mapping: not valid YAML, forces FormatFile's error branch
	// without relying on filesystem permission tricks.
	require.NoError(t, os.WriteFile(file, []byte("settings: {enabled: true\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))

	err = configFormatCmd.RunE(configFormatCmd, nil)
	require.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
}

func TestResolveConfigFile_DiscoversFromCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("settings:\n  enabled: true\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))

	// No "config" flag registered at all, so cmd.Flags().GetStringSlice("config")
	// errors and override stays "", forcing the discovery path.
	cmd := &cobra.Command{}

	got, err := resolveConfigFile(cmd)
	require.NoError(t, err)

	wantResolved, err := filepath.EvalSymlinks(file)
	require.NoError(t, err)
	gotResolved, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	assert.Equal(t, wantResolved, gotResolved)
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
