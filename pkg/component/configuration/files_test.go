package configuration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWriteFilesOwnershipAndLiteralContent(t *testing.T) {
	root := t.TempDir()
	file := schema.ComponentConfigurationFile{Path: "nested/context.tf", Content: []byte(`{{ do not render }} ${terraform.expression}`), IfExists: "error"}
	require.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{file}))
	path := filepath.Join(root, file.Path)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, file.Content, content)
	require.ErrorIs(t, WriteFiles(root, []schema.ComponentConfigurationFile{file}), errUtils.ErrPluginInvalidConfig)
	file.Content = []byte("replacement")
	file.IfExists = "skip"
	require.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{file}))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	require.NotEqual(t, file.Content, content)
	file.IfExists = "overwrite"
	file.Sensitive = true
	require.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{file}))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, file.Content, content)
	stat, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), stat.Mode().Perm())
	}
	require.NoError(t, WriteFiles("unavailable", nil))
	require.ErrorIs(t, WriteFiles(filepath.Join(root, "absent"), []schema.ComponentConfigurationFile{file}), errUtils.ErrPluginInvalidConfig)
}

func TestWriteFilesPreflightAndContainment(t *testing.T) {
	root := t.TempDir()
	first := schema.ComponentConfigurationFile{Path: "first.tf", Content: []byte("content"), IfExists: "error"}
	for _, file := range []schema.ComponentConfigurationFile{{Path: "../escape.tf", IfExists: "overwrite"}, {Path: "bad.tf", IfExists: "unknown"}} {
		require.Error(t, WriteFiles(root, []schema.ComponentConfigurationFile{first, file}))
		_, err := os.Stat(filepath.Join(root, first.Path))
		require.True(t, os.IsNotExist(err))
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	require.Error(t, WriteFiles(root, []schema.ComponentConfigurationFile{{Path: "link/escape.tf", IfExists: "overwrite"}}))
	_, err := os.Stat(filepath.Join(outside, "escape.tf"))
	require.True(t, os.IsNotExist(err))
}

func TestWatchPaths(t *testing.T) {
	root := t.TempDir()
	for _, provider := range []bool{true, false} {
		settings := map[string]any{}
		if provider {
			settings["configuration"] = map[string]any{"provider": "terragrunt"}
		}
		absolute := filepath.Join(root, "module")
		deps := []schema.ComponentDependency{{Path: "root.hcl"}, {Path: absolute}}
		require.Equal(t, provider, WatchPaths(&schema.AtmosConfiguration{BasePathAbsolute: root}, settings, deps))
		require.Equal(t, absolute, deps[1].Path)
		if provider {
			require.Equal(t, filepath.Join(root, "root.hcl"), deps[0].Path)
		} else {
			require.Equal(t, "root.hcl", deps[0].Path)
		}
	}
}
