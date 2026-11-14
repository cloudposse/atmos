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

func TestTerragruntOwnershipPolicies(t *testing.T) {
	for _, policy := range []string{"overwrite_terragrunt", "remove_terragrunt", "remove"} {
		for _, existing := range []string{"", "unsigned", "# " + terragruntSignature, "// " + terragruntSignature + "\nold"} {
			t.Run(policy+"/"+existing, func(t *testing.T) {
				directory := t.TempDir()
				path := filepath.Join(directory, "generated.tf")
				if existing != "" {
					require.NoError(t, os.WriteFile(path, []byte(existing), 0o600))
				}
				file := schema.ComponentConfigurationFile{Path: "generated.tf", Content: []byte("replacement"), IfExists: policy}
				err := WriteFiles(directory, []schema.ComponentConfigurationFile{file})
				if existing == "unsigned" && policy != "remove" {
					require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
					contents, err := os.ReadFile(path)
					require.NoError(t, err)
					require.Equal(t, existing, string(contents))
					return
				}
				require.NoError(t, err)
				if policy == "overwrite_terragrunt" {
					contents, err := os.ReadFile(path)
					require.NoError(t, err)
					require.Equal(t, "replacement", string(contents))
				} else {
					require.NoFileExists(t, path)
				}
			})
		}
	}
}

func TestGeneratedOwnershipRejectsSymlinksAndDirectories(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "directory.tf"), 0o700))
	require.Error(t, WriteFiles(root, []schema.ComponentConfigurationFile{{Path: "directory.tf", IfExists: "remove"}}))
	path := filepath.Join(root, "target.tf")
	original := "# " + terragruntSignature + "\nkeep"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	if err := os.Symlink(path, filepath.Join(root, "link.tf")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, policy := range []string{"overwrite", "overwrite_terragrunt", "remove", "remove_terragrunt"} {
		require.Error(t, WriteFiles(root, []schema.ComponentConfigurationFile{{Path: "link.tf", IfExists: policy}}))
	}
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(contents))
}

func TestTerragruntOwnershipFailsClosedWhenSignatureCannotBeRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions are not enforced on Windows")
	}
	for _, policy := range []string{"overwrite_terragrunt", "remove_terragrunt"} {
		t.Run(policy, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "generated.tf")
			original := "# " + terragruntSignature + "\nkeep"
			require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
			require.NoError(t, os.Chmod(path, 0))
			t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
			if file, err := os.Open(path); err == nil {
				_ = file.Close()
				t.Skip("permissions are not enforced for this user")
			}
			err := WriteFiles(directory, []schema.ComponentConfigurationFile{{Path: "generated.tf", Content: []byte("replacement"), IfExists: policy}})
			require.ErrorIs(t, err, os.ErrPermission, "ownership that cannot be proven must not allow replacement or removal")
			require.NoError(t, os.Chmod(path, 0o600))
			contents, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, string(contents))
		})
	}
}
