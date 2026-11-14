package autoinit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvidersMissingSelectedInstallation(t *testing.T) {
	for _, tc := range []struct {
		name, layout, damage string
		missing              bool
	}{
		{"complete", "providers", "", false},
		{"unversioned executable in versioned package", "providers", "unversioned", false},
		{"empty package", "providers", "missing", true},
		{"wrong version", "providers", "version", true},
		{"other platform", "providers", "platform", true},
		{"directory named executable", "providers", "directory", true},
		{"broken executable symlink", "providers", "symlink", true},
		{"valid executable symlink", "providers", "valid-symlink", false},
		{"broken package symlink", "providers", "package-symlink", true},
		{"nonexecutable file", "providers", "nonexecutable", true},
		{"legacy complete", "plugins", "", false},
		{"legacy empty", "plugins", "missing", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			data := filepath.Join(t.TempDir(), "custom-data")
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), []byte(validLockFile), 0o600))
			version := "3.2.1"
			platform := runtime.GOOS + "_" + runtime.GOARCH
			if tc.damage == "version" {
				version = "3.2.10"
			}
			if tc.damage == "platform" {
				platform = "other_arch"
			}
			installed := filepath.Join(data, tc.layout, "registry.terraform.io", "hashicorp", "null", version, platform)
			if tc.layout == "plugins" {
				installed = filepath.Join(data, tc.layout, platform)
			}
			require.NoError(t, os.MkdirAll(installed, 0o755))
			name := "terraform-provider-null_v" + version + "_x5"
			if tc.damage == "unversioned" {
				name = "terraform-provider-null"
			}
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			executable := filepath.Join(installed, name)
			switch tc.damage {
			case "missing":
			case "directory":
				require.NoError(t, os.Mkdir(executable, 0o755))
			case "package-symlink":
				if runtime.GOOS == "windows" {
					t.Skip("symlink creation requires additional Windows privileges")
				}
				require.NoError(t, os.Remove(installed))
				require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "missing"), installed))
			case "valid-symlink":
				if runtime.GOOS == "windows" {
					t.Skip("symlink creation requires additional Windows privileges")
				}
				target := filepath.Join(t.TempDir(), "provider")
				require.NoError(t, os.WriteFile(target, []byte("provider"), 0o755))
				require.NoError(t, os.Symlink(target, executable))
			case "nonexecutable":
				if runtime.GOOS == "windows" {
					t.Skip("Windows does not use POSIX executable permissions")
				}
				require.NoError(t, os.WriteFile(executable, []byte("provider"), 0o644))
			case "symlink":
				if runtime.GOOS == "windows" {
					t.Skip("symlink creation requires additional Windows privileges")
				}
				require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "missing"), executable))
			default:
				require.NoError(t, os.WriteFile(executable, []byte("provider"), 0o755))
			}
			assert.Equal(t, tc.missing, providersMissing(data, dir))
		})
	}
}

func TestProviderInstallDiagnosticDoesNotUpgrade(t *testing.T) {
	for _, output := range []string{
		"cached package does not match any of the checksums recorded in the dependency lock file, run tofu init to ensure all providers are correctly installed.",
		"cached package does not match any of the checksums recorded in the dependency lock file, run terraform init to ensure all providers are correctly installed.",
	} {
		diagnosis := Classify(output)
		assert.True(t, diagnosis.InitRequired)
		assert.False(t, diagnosis.UpgradeRequired)
		assert.False(t, diagnosis.ReconfigureRequired)
	}
	for _, output := range []string{"Failed to load plugin schemas: permission denied", "provider startup: connection refused", "Error acquiring the state lock", "AccessDenied: expired credentials"} {
		assert.False(t, Classify(output).InitRequired)
	}
}
