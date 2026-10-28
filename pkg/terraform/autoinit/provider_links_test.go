package autoinit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/terraform/lockfile"
)

func TestPrepareProviderInstallation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional Windows privileges")
	}
	for _, kind := range []string{"dangling", "live", "directory", "other-version"} {
		t.Run(kind, func(t *testing.T) {
			componentPath := t.TempDir()
			data := filepath.Join(t.TempDir(), "custom-data")
			require.NoError(t, os.WriteFile(filepath.Join(componentPath, lockfile.Name), []byte(validLockFile), 0o600))
			version := "3.2.1"
			if kind == "other-version" {
				version = "3.2.0"
			}
			packagePath := filepath.Join(data, "providers", "registry.terraform.io", "hashicorp", "null", version, runtime.GOOS+"_"+runtime.GOARCH)
			require.NoError(t, os.MkdirAll(filepath.Dir(packagePath), 0o755))
			target := filepath.Join(t.TempDir(), "target")
			if kind == "directory" {
				require.NoError(t, os.Mkdir(packagePath, 0o755))
			} else {
				if kind == "live" {
					require.NoError(t, os.Mkdir(target, 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(target, "sentinel"), []byte("preserve"), 0o600))
					require.NoError(t, os.WriteFile(filepath.Join(target, "terraform-provider-null_v3.2.1_x5"), []byte("provider"), 0o755))
				}
				require.NoError(t, os.Symlink(target, packagePath))
			}
			in := &Inputs{ComponentPath: componentPath, DataDir: data}
			_, err := PrepareProviderInstallation(in)
			require.NoError(t, err)
			_, err = PrepareProviderInstallation(in)
			require.NoError(t, err)
			_, err = os.Lstat(packagePath)
			if kind == "dangling" || kind == "directory" {
				assert.True(t, os.IsNotExist(err))
			} else {
				require.NoError(t, err)
			}
			if kind == "live" {
				b, e := os.ReadFile(filepath.Join(target, "sentinel"))
				require.NoError(t, e)
				assert.Equal(t, "preserve", string(b))
			}
		})
	}
	_, err := PrepareProviderInstallation(nil)
	require.NoError(t, err)
	_, err = PrepareProviderInstallation(&Inputs{ComponentPath: t.TempDir()})
	require.NoError(t, err)
}

func TestProviderPackageDirsRejectTraversal(t *testing.T) {
	for _, provider := range []lockfile.Provider{
		{Source: "../hashicorp/null", Version: "1.0"},
		{Source: "registry.example.com/hashicorp/null", Version: "../../elsewhere"},
		{Source: "registry.example.com/hashicorp/null", Version: `..\elsewhere`},
		{Source: "registry.example.com/hashicorp", Version: "1.0"},
		{Source: "registry.example.com//null", Version: "1.0"},
	} {
		assert.Empty(t, providerPackageDirs(t.TempDir(), provider))
		assert.False(t, providerInstalled(t.TempDir(), provider))
	}
}

func TestPrepareProviderInstallationPreservesIncompleteSharedPackage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional Windows privileges")
	}
	componentPath := t.TempDir()
	data := filepath.Join(componentPath, ".terraform")
	cache := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, lockfile.Name), []byte(validLockFile), 0o600))
	relative := filepath.Join("registry.terraform.io", "hashicorp", "null", "3.2.1", runtime.GOOS+"_"+runtime.GOARCH)
	shared := filepath.Join(cache, relative)
	local := filepath.Join(data, "providers", relative)
	require.NoError(t, os.MkdirAll(shared, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(local), 0o755))
	sentinel := filepath.Join(shared, "LICENSE")
	require.NoError(t, os.WriteFile(sentinel, []byte("preserved"), 0o600))
	require.NoError(t, os.Symlink(shared, local))
	in := &Inputs{ComponentPath: componentPath, DataDir: data, EnvLookup: func(key string) (string, bool) { return cache, key == envTFPluginCacheDir }}
	recovery, err := PrepareProviderInstallation(in)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(data, "atmos-provider-recovery"), recovery)
	_, err = os.Lstat(local)
	assert.True(t, os.IsNotExist(err))
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "preserved", string(content))
	// A second attempt still bypasses the unchanged broken shared package even after unlinking it.
	again, err := PrepareProviderInstallation(in)
	require.NoError(t, err)
	assert.Equal(t, recovery, again)
}

func TestPreserveIncompletePackageOwnership(t *testing.T) {
	data := t.TempDir()
	local := filepath.Join(data, "providers", "partial")
	require.NoError(t, os.MkdirAll(local, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(local, "LICENSE"), []byte("preserve exact content"), 0o600))
	require.NoError(t, preserveIncompletePackage(data, local))
	matches, err := filepath.Glob(filepath.Join(data, "atmos-incomplete-provider-*", "package", "LICENSE"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	content, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	assert.Equal(t, "preserve exact content", string(content))
	if runtime.GOOS == "windows" {
		return
	}
	shared := t.TempDir()
	outside := filepath.Join(shared, "partial")
	require.NoError(t, os.Mkdir(outside, 0o755))
	viaLink := filepath.Join(data, "shared")
	require.NoError(t, os.Symlink(shared, viaLink))
	require.NoError(t, preserveIncompletePackage(data, filepath.Join(viaLink, "partial")))
	_, err = os.Stat(outside)
	require.NoError(t, err, "must not move shared-cache contents through an ancestor symlink")
}

func TestProviderRecoveryFilesystemFailuresPreservePackages(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires non-root POSIX permissions")
	}
	for _, mode := range []string{"unreadable", "quarantine-create", "quarantine-rename"} {
		t.Run(mode, func(t *testing.T) {
			data := t.TempDir()
			parent := filepath.Join(data, "providers")
			path := filepath.Join(parent, "package")
			require.NoError(t, os.MkdirAll(path, 0o755))
			sentinel := filepath.Join(path, "LICENSE")
			require.NoError(t, os.WriteFile(sentinel, []byte("original"), 0o600))
			protected := data
			if mode == "unreadable" {
				protected = path
			}
			if mode == "quarantine-rename" {
				protected = parent
			}
			permission := os.FileMode(0o500)
			if mode == "unreadable" {
				permission = 0
			}
			require.NoError(t, os.Chmod(protected, permission))
			t.Cleanup(func() { _ = os.Chmod(protected, 0o755) })
			needed, err := prepareLocalPackage(data, path, "terraform-provider-null_v3.2.1", true)
			assert.False(t, needed)
			if mode == "unreadable" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errUtils.ErrTerraformInit)
			}
			require.NoError(t, os.Chmod(protected, 0o755))
			content, readErr := os.ReadFile(sentinel)
			require.NoError(t, readErr)
			assert.Equal(t, "original", string(content))
		})
	}
}

func TestProviderRecoveryCacheCreationFailure(t *testing.T) {
	componentPath := t.TempDir()
	data := filepath.Join(componentPath, ".terraform")
	cache := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, lockfile.Name), []byte(validLockFile), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(cache, "registry.terraform.io", "hashicorp", "null", "3.2.1", runtime.GOOS+"_"+runtime.GOARCH), 0o755))
	require.NoError(t, os.MkdirAll(data, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(data, "atmos-provider-recovery"), []byte("reserved"), 0o600))
	in := &Inputs{ComponentPath: componentPath, DataDir: data, EnvLookup: func(key string) (string, bool) { return cache, key == envTFPluginCacheDir }}
	_, err := PrepareProviderInstallation(in)
	require.ErrorIs(t, err, errUtils.ErrTerraformInit)
}

func TestProviderRecoveryRejectsReservedDirectorySymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional Windows privileges")
	}
	componentPath := t.TempDir()
	data := filepath.Join(componentPath, ".terraform")
	cache := t.TempDir()
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, lockfile.Name), []byte(validLockFile), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(cache, "registry.terraform.io", "hashicorp", "null", "3.2.1", runtime.GOOS+"_"+runtime.GOARCH), 0o755))
	require.NoError(t, os.MkdirAll(data, 0o755))
	require.NoError(t, os.Symlink(external, filepath.Join(data, "atmos-provider-recovery")))
	sentinel := filepath.Join(external, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("unchanged"), 0o600))
	in := &Inputs{ComponentPath: componentPath, DataDir: data, EnvLookup: func(key string) (string, bool) { return cache, key == envTFPluginCacheDir }}
	_, err := PrepareProviderInstallation(in)
	require.ErrorIs(t, err, errUtils.ErrTerraformInit)
	assert.Contains(t, err.Error(), "must not be a symlink")
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", string(content))
	entries, err := os.ReadDir(external)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "sentinel", entries[0].Name())
}

func TestProviderRecoveryInspectionFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires non-root POSIX directory permissions")
	}
	data := t.TempDir()
	parent := filepath.Join(data, "inaccessible")
	require.NoError(t, os.Mkdir(parent, 0o700))
	sentinel := filepath.Join(parent, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("untouched"), 0o600))
	require.NoError(t, os.Chmod(parent, 0))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	_, err := prepareLocalPackage(data, filepath.Join(parent, "package"), "terraform-provider-null_v3.2.1", true)
	require.ErrorIs(t, err, errUtils.ErrTerraformInit)
	require.NoError(t, os.Chmod(parent, 0o700))
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "untouched", string(content))
}

func TestPrepareProviderInstallationPropagatesInspectionFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires non-root POSIX directory permissions")
	}
	root := t.TempDir()
	data := filepath.Join(root, ".terraform")
	blocked := filepath.Join(data, "providers")
	require.NoError(t, os.WriteFile(filepath.Join(root, lockfile.Name), []byte(validLockFile), 0o600))
	require.NoError(t, os.MkdirAll(blocked, 0o700))
	sentinel := filepath.Join(blocked, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0o600))
	require.NoError(t, os.Chmod(blocked, 0))
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
	cache, err := PrepareProviderInstallation(&Inputs{ComponentPath: root, DataDir: data})
	require.ErrorIs(t, err, errUtils.ErrTerraformInit)
	assert.Empty(t, cache)
	require.NoError(t, os.Chmod(blocked, 0o700))
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "preserve", string(content))
}

func TestPrepareProviderInstallationRejectsUnsafeLockAddresses(t *testing.T) {
	for _, source := range []string{"broken", "../hashicorp/null"} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			data := filepath.Join(root, ".terraform")
			require.NoError(t, os.WriteFile(filepath.Join(root, lockfile.Name), []byte("provider \""+source+"\" {\n version = \"3.2.1\"\n}\n"), 0o600))
			cache, err := PrepareProviderInstallation(&Inputs{ComponentPath: root, DataDir: data})
			require.NoError(t, err)
			assert.Empty(t, cache)
			assert.NoDirExists(t, data, "invalid addresses must not create or repair installations")
		})
	}
}

func TestPrepareProviderInstallationUnlinkFailurePreservesSharedTarget(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires non-root POSIX directory permissions")
	}
	root := t.TempDir()
	data := filepath.Join(root, ".terraform")
	shared := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, lockfile.Name), []byte(validLockFile), 0o600))
	local := filepath.Join(data, "providers", "registry.terraform.io", "hashicorp", "null", "3.2.1", runtime.GOOS+"_"+runtime.GOARCH)
	parent := filepath.Dir(local)
	require.NoError(t, os.MkdirAll(parent, 0o755))
	require.NoError(t, os.Symlink(shared, local))
	sentinel := filepath.Join(shared, "LICENSE")
	require.NoError(t, os.WriteFile(sentinel, []byte("shared content"), 0o600))
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	_, err := PrepareProviderInstallation(&Inputs{ComponentPath: root, DataDir: data})
	require.ErrorIs(t, err, errUtils.ErrTerraformInit)
	target, err := os.Readlink(local)
	require.NoError(t, err)
	assert.Equal(t, shared, target)
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "shared content", string(content))
}
