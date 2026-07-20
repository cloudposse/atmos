package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveImportBasePath(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name      string
		basePath  string
		configDir string
		source    string
		want      string
	}{
		{"empty base path anchors to config dir", "", dir, "", dir},
		{"dot anchors to config dir", ".", dir, "", dir},
		{"dot-slash anchors to config dir", "./", dir, "", dir},
		{"dot-dot anchors to config dir", "..", dir, "", filepath.Dir(dir)},
		{"runtime dot retains CWD resolution", ".", dir, basePathSourceRuntime, "."},
		{"bare base path retains downstream resolution", "sub", dir, "", "sub"},
		{"nested bare base path retains downstream resolution", filepath.Join("a", "b"), dir, "", filepath.Join("a", "b")},
		{"absolute base path is returned unchanged", dir, filepath.Join(dir, "unused"), "", dir},
		{"empty config dir falls back to base path", ".", "", "", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveImportBasePath(tt.basePath, tt.configDir, tt.source))
		})
	}
}

func TestMergeImports_ResolvesImportsRelativeToConfigDir(t *testing.T) {
	setupTestAdapters()
	root := t.TempDir()
	importDir := filepath.Join(root, ".atmos")
	require.NoError(t, os.MkdirAll(importDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(importDir, "extra.yaml"),
		[]byte("settings:\n  terminal:\n    max_width: 137\n"),
		0o644,
	))

	t.Chdir(t.TempDir())

	v := viper.New()
	v.SetConfigType(yamlType)
	v.Set("base_path", ".")
	v.Set("import", []string{filepath.Join(".atmos", "extra.yaml")})

	_, err := mergeImports(v, root, "")
	require.NoError(t, err)

	assert.Equal(t, 137, v.GetInt("settings.terminal.max_width"),
		"import must resolve relative to the config dir, not the cwd")
}

func TestMergeConfig_GlobRelativeToConfigDir(t *testing.T) {
	setupTestAdapters()
	root := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", root)
	commandsDir := filepath.Join(root, ".atmos", "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(commandsDir, "extra.yaml"),
		[]byte("settings:\n  terminal:\n    max_width: 141\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "atmos.yaml"),
		[]byte("base_path: ./\nimport:\n  - .atmos/commands/**/*\n"),
		0o644,
	))

	sub := filepath.Join(root, "somepath")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	v := viper.New()
	v.SetConfigType(yamlType)
	require.NoError(t, mergeConfig(v, root, CliConfigFileName, true))

	assert.Equal(t, 141, v.GetInt("settings.terminal.max_width"),
		"glob import must resolve relative to the config dir, not the cwd")
}

func TestMergeImports_BareBasePathRetainsCWDResolution(t *testing.T) {
	setupTestAdapters()
	configDir := t.TempDir()
	cwd := t.TempDir()
	importDir := filepath.Join(cwd, "workspace")
	require.NoError(t, os.MkdirAll(importDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(importDir, "extra.yaml"),
		[]byte("settings:\n  terminal:\n    max_width: 149\n"),
		0o644,
	))
	t.Chdir(cwd)

	v := viper.New()
	v.SetConfigType(yamlType)
	v.Set("base_path", "workspace")
	v.Set("import", []string{"extra.yaml"})

	_, err := mergeImports(v, configDir, "")
	require.NoError(t, err)
	assert.Equal(t, 149, v.GetInt("settings.terminal.max_width"))
}

func TestMergeConfig_CwdBasePathRetainsCWDImportResolution(t *testing.T) {
	setupTestAdapters()
	configDir := t.TempDir()
	cwd := t.TempDir()
	importDir := filepath.Join(cwd, "workspace")
	require.NoError(t, os.MkdirAll(importDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(importDir, "extra.yaml"),
		[]byte("settings:\n  terminal:\n    max_width: 151\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "atmos.yaml"),
		[]byte("base_path: !cwd ./workspace\nimport:\n  - extra.yaml\n"),
		0o644,
	))
	t.Chdir(cwd)

	v := viper.New()
	v.SetConfigType(yamlType)
	require.NoError(t, mergeConfig(v, configDir, CliConfigFileName, true))
	assert.Equal(t, 151, v.GetInt("settings.terminal.max_width"))
}

func TestMergeFiles_InheritedBasePathUsesDeclaringConfigDir(t *testing.T) {
	setupTestAdapters()
	baseConfigDir := t.TempDir()
	overlayConfigDir := t.TempDir()
	importDir := filepath.Join(baseConfigDir, "workspace")
	require.NoError(t, os.MkdirAll(importDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(importDir, "extra.yaml"),
		[]byte("settings:\n  terminal:\n    max_width: 157\n"),
		0o644,
	))
	baseConfig := filepath.Join(baseConfigDir, "base.yaml")
	require.NoError(t, os.WriteFile(baseConfig, []byte("base_path: ./workspace\n"), 0o644))
	overlayConfig := filepath.Join(overlayConfigDir, "overlay.yaml")
	require.NoError(t, os.WriteFile(overlayConfig, []byte("import:\n  - extra.yaml\n"), 0o644))

	v := viper.New()
	v.SetConfigType(yamlType)
	require.NoError(t, mergeFiles(v, []string{baseConfig, overlayConfig}))
	assert.Equal(t, 157, v.GetInt("settings.terminal.max_width"))
}

func TestMergeFiles_ImportedBasePathUsesImportedConfigDir(t *testing.T) {
	setupTestAdapters()
	baseConfigDir := t.TempDir()
	overlayConfigDir := t.TempDir()
	defaultsDir := filepath.Join(baseConfigDir, "imports")
	importDir := filepath.Join(baseConfigDir, "workspace")
	require.NoError(t, os.MkdirAll(defaultsDir, 0o755))
	require.NoError(t, os.MkdirAll(importDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(importDir, "extra.yaml"),
		[]byte("settings:\n  terminal:\n    max_width: 163\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(filepath.Join(defaultsDir, "defaults.yaml"), []byte("base_path: ../workspace\n"), 0o644))
	baseConfig := filepath.Join(baseConfigDir, "base.yaml")
	require.NoError(t, os.WriteFile(baseConfig, []byte("import:\n  - imports/defaults.yaml\n"), 0o644))
	overlayConfig := filepath.Join(overlayConfigDir, "overlay.yaml")
	require.NoError(t, os.WriteFile(overlayConfig, []byte("import:\n  - extra.yaml\n"), 0o644))

	v := viper.New()
	v.SetConfigType(yamlType)
	require.NoError(t, mergeFiles(v, []string{baseConfig, overlayConfig}))
	assert.Equal(t, 163, v.GetInt("settings.terminal.max_width"))
}

func TestMergeFiles_DirectBasePathKeepsPrecedenceOverImportedDeclaration(t *testing.T) {
	setupTestAdapters()
	baseConfigDir := t.TempDir()
	overlayConfigDir := t.TempDir()
	directImportDir := filepath.Join(baseConfigDir, "direct")
	defaultsDir := filepath.Join(baseConfigDir, "imports")
	require.NoError(t, os.MkdirAll(directImportDir, 0o755))
	require.NoError(t, os.MkdirAll(defaultsDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(directImportDir, "extra.yaml"),
		[]byte("settings:\n  terminal:\n    max_width: 167\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(filepath.Join(defaultsDir, "defaults.yaml"), []byte("base_path: ../imported\n"), 0o644))
	baseConfig := filepath.Join(baseConfigDir, "base.yaml")
	require.NoError(t, os.WriteFile(baseConfig, []byte("base_path: ./direct\nimport:\n  - imports/defaults.yaml\n"), 0o644))
	overlayConfig := filepath.Join(overlayConfigDir, "overlay.yaml")
	require.NoError(t, os.WriteFile(overlayConfig, []byte("import:\n  - extra.yaml\n"), 0o644))

	v := viper.New()
	v.SetConfigType(yamlType)
	require.NoError(t, mergeFiles(v, []string{baseConfig, overlayConfig}))
	assert.Equal(t, 167, v.GetInt("settings.terminal.max_width"))
}

func TestMergeFiles_EmptyBasePathUsesImportingConfigDir(t *testing.T) {
	setupTestAdapters()
	baseConfigDir := t.TempDir()
	overlayConfigDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(baseConfigDir, "base.yaml"), []byte("settings:\n  base: true\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(overlayConfigDir, "extra.yaml"),
		[]byte("settings:\n  terminal:\n    max_width: 173\n"),
		0o644,
	))
	overlayConfig := filepath.Join(overlayConfigDir, "overlay.yaml")
	require.NoError(t, os.WriteFile(overlayConfig, []byte("import:\n  - extra.yaml\n"), 0o644))

	v := viper.New()
	v.SetConfigType(yamlType)
	require.NoError(t, mergeFiles(v, []string{filepath.Join(baseConfigDir, "base.yaml"), overlayConfig}))
	assert.Equal(t, 173, v.GetInt("settings.terminal.max_width"))
}
