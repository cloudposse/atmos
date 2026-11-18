package configuration

import (
	"fmt"
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

func TestWriteFilesAppliesClampedMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores POSIX permission bits.")
	}
	tests := []struct {
		name      string
		mode      uint32
		sensitive bool
		want      os.FileMode
		// umaskDefault expects the creation default (0644) as the process umask leaves it.
		umaskDefault bool
	}{
		{name: "absent mode keeps the default", mode: 0, umaskDefault: true},
		{name: "executable keeps the execute bit", mode: 0o755, want: 0o755},
		{name: "owner-only executable", mode: 0o700, want: 0o700},
		{name: "group and other write are stripped", mode: 0o777, want: 0o755},
		{name: "setuid setgid sticky are stripped", mode: 0o7755, want: 0o755},
		{name: "owner-only file stays private", mode: 0o600, want: 0o600},
		{name: "group-readable file is not widened", mode: 0o640, want: 0o640},
		{name: "owner read-write is guaranteed", mode: 0o111, want: 0o711},
		{name: "sensitive stays private", mode: 0o755, sensitive: true, want: 0o700},
		{name: "sensitive without mode", mode: 0, sensitive: true, want: 0o600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			file := schema.ComponentConfigurationFile{Path: filepath.Join("bin", "helper.sh"), Content: []byte("#!/bin/sh\n"), IfExists: "overwrite", Mode: tt.mode, Sensitive: tt.sensitive}
			require.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{file}))
			stat, err := os.Stat(filepath.Join(root, file.Path))
			require.NoError(t, err)
			want := tt.want
			if tt.umaskDefault {
				want = umaskedDefaultMode(t)
			}
			require.Equal(t, want, stat.Mode())
		})
	}
	t.Run("overwrite updates the mode of an existing file", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "run.sh")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
		require.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{{Path: "run.sh", Content: []byte("new"), IfExists: "overwrite", Mode: 0o755}}))
		stat, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o755), stat.Mode())
	})
}

// umaskedDefaultMode creates an independent 0644 reference file under the current
// process umask and returns the mode the filesystem granted it. An absent mode skips
// chmod, so the writer must match this rather than a fixed 0644.
func umaskedDefaultMode(t *testing.T) os.FileMode {
	t.Helper()
	reference := filepath.Join(t.TempDir(), "reference")
	output, err := os.OpenFile(reference, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	require.NoError(t, err)
	require.NoError(t, output.Close())
	stat, err := os.Stat(reference)
	require.NoError(t, err)
	return stat.Mode()
}

func TestWriteFilesRejectsModesBeyondPermissionBits(t *testing.T) {
	root := t.TempDir()
	err := WriteFiles(root, []schema.ComponentConfigurationFile{{Path: "a.sh", IfExists: "error", Mode: 1 << 31}})
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	_, statErr := os.Stat(filepath.Join(root, "a.sh"))
	require.True(t, os.IsNotExist(statErr))
}

func TestWriteFilesRejectsRuntimeAliases(t *testing.T) {
	for _, directoryAlias := range []bool{false, true} {
		t.Run(fmt.Sprintf("directory=%v", directoryAlias), func(t *testing.T) {
			root := t.TempDir()
			target := "terraform.tfstate"
			alias := "alias.tf"
			require.NoError(t, os.WriteFile(filepath.Join(root, target), []byte("state"), 0o600))
			if directoryAlias {
				require.NoError(t, os.Mkdir(filepath.Join(root, ".terraform"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, ".terraform", "environment"), []byte("stage"), 0o600))
				target, alias = ".terraform", "alias"
			}
			if err := os.Symlink(target, filepath.Join(root, alias)); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			path := alias
			if directoryAlias {
				path = filepath.Join(alias, "environment")
			}
			file := schema.ComponentConfigurationFile{Path: path, Content: []byte("destroyed"), IfExists: "overwrite"}
			err := WriteFiles(root, []schema.ComponentConfigurationFile{{Path: "first.tf", IfExists: "overwrite"}, file})
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.NoFileExists(t, filepath.Join(root, "first.tf"), "preflight covers all files before writing")
			expected := "state"
			if directoryAlias {
				target = filepath.Join(target, "environment")
				expected = "stage"
			}
			content, err := os.ReadFile(filepath.Join(root, target))
			require.NoError(t, err)
			require.Equal(t, expected, string(content))
		})
	}
}

func TestWriteFileRejectsAliasIntroducedAfterPreflight(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	defer root.Close()
	file := schema.ComponentConfigurationFile{Path: "generated.tf", Content: []byte("destroyed"), IfExists: "overwrite"}
	require.NoError(t, validateFile(file))
	require.NoError(t, validateDestination(root, file.Path))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "terraform.tfstate"), []byte("state"), 0o600))
	if err := os.Symlink("terraform.tfstate", filepath.Join(directory, file.Path)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	require.ErrorIs(t, writeFile(root, file), errUtils.ErrPluginInvalidConfig)
	content, err := root.ReadFile("terraform.tfstate")
	require.NoError(t, err)
	require.Equal(t, "state", string(content))
}

func TestWriteFilesOverwritePreservesOtherHardLinks(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "terraform.tfstate")
	generated := filepath.Join(root, "generated.tf")
	require.NoError(t, os.WriteFile(state, []byte("protected state"), 0o600))
	require.NoError(t, os.Link(state, generated))
	require.NoError(t, WriteFiles(root, []schema.ComponentConfigurationFile{{Path: "generated.tf", Content: []byte("replacement"), IfExists: "overwrite", Mode: 0o755}}))
	content, err := os.ReadFile(state)
	require.NoError(t, err)
	require.Equal(t, "protected state", string(content))
	content, err = os.ReadFile(generated)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(content))
	oldInfo, err := os.Stat(state)
	require.NoError(t, err)
	newInfo, err := os.Stat(generated)
	require.NoError(t, err)
	require.False(t, os.SameFile(oldInfo, newInfo))
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), oldInfo.Mode().Perm())
	}
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 2, "temporary replacement is removed")
}
