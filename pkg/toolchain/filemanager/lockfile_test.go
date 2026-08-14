package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroachdberrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/lockfile"
)

func TestLockFileManager_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected bool
	}{
		{
			name: "enabled",
			config: &schema.AtmosConfiguration{
				Toolchain: schema.Toolchain{
					UseLockFile: true,
				},
			},
			expected: true,
		},
		{
			name: "disabled",
			config: &schema.AtmosConfiguration{
				Toolchain: schema.Toolchain{
					UseLockFile: false,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewLockFileManager(tt.config)
			assert.Equal(t, tt.expected, mgr.Enabled())
		})
	}
}

func TestLockFileManager_AddTool(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Add tool with metadata
	err := mgr.AddTool(
		ctx, "hashicorp/terraform", "1.13.4",
		WithURL("https://example.com/terraform.zip"),
		WithChecksum("sha256:abc123"),
		WithSize(95842304),
	)
	require.NoError(t, err)

	// Verify file exists and contains tool
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "hashicorp/terraform")
	assert.Contains(t, string(content), "1.13.4")
	assert.Contains(t, string(content), "sha256:abc123")
	assert.Contains(t, string(content), "https://example.com/terraform.zip")
}

func TestLockFileManager_AddTool_Disabled(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: false,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Add tool (should be skipped)
	err := mgr.AddTool(ctx, "hashicorp/terraform", "1.13.4")
	require.NoError(t, err)

	// Verify file was NOT created
	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err))
}

// TestLockFileManager_AddTool_RetainsBothVersions locks two different versions of the same
// tool and confirms both survive independently. A prior version of this test asserted the
// opposite -- that locking a second version discarded the first -- which was itself the exact
// silent-data-loss bug this schema fix addresses (see lockfile.Tool.Versions): a
// .tool-versions line can legitimately pin more than one version of the same tool (e.g.
// "yq 4.45.1 4.50.1"), so locking a second version must never clobber the first's checksum.
func TestLockFileManager_AddTool_RetainsBothVersions(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Lock first version
	err := mgr.AddTool(
		ctx, "hashicorp/terraform", "1.10.0",
		WithURL("https://example.com/terraform-1.10.0.zip"),
		WithChecksum("sha256:old123"),
	)
	require.NoError(t, err)

	// Lock a second version
	err = mgr.AddTool(
		ctx, "hashicorp/terraform", "1.13.4",
		WithURL("https://example.com/terraform-1.13.4.zip"),
		WithChecksum("sha256:new123"),
	)
	require.NoError(t, err)

	// Verify the file retains both versions' data.
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "1.13.4")
	assert.Contains(t, string(content), "sha256:new123")
	assert.Contains(t, string(content), "1.10.0")
	assert.Contains(t, string(content), "sha256:old123")
}

func TestLockFileManager_RemoveTool(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Add two tools
	err := mgr.AddTool(
		ctx, "hashicorp/terraform", "1.13.4",
		WithURL("https://example.com/terraform.zip"),
		WithChecksum("sha256:abc123"),
	)
	require.NoError(t, err)

	err = mgr.AddTool(
		ctx, "kubernetes/kubectl", "1.34.1",
		WithURL("https://example.com/kubectl.zip"),
		WithChecksum("sha256:def456"),
	)
	require.NoError(t, err)

	// Remove one tool
	err = mgr.RemoveTool(ctx, "hashicorp/terraform", "1.13.4")
	require.NoError(t, err)

	// Verify file no longer contains removed tool
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "hashicorp/terraform")
	assert.Contains(t, string(content), "kubernetes/kubectl")
}

func TestLockFileManager_SetDefault(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Set default (should update version)
	err := mgr.SetDefault(ctx, "hashicorp/terraform", "1.13.4")
	require.NoError(t, err)

	// Verify file contains version
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "1.13.4")
}

func TestLockFileManager_GetTools(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Add multiple tools
	err := mgr.AddTool(ctx, "hashicorp/terraform", "1.13.4")
	require.NoError(t, err)

	err = mgr.AddTool(ctx, "kubernetes/kubectl", "1.34.1")
	require.NoError(t, err)

	// Get tools
	tools, err := mgr.GetTools(ctx)
	require.NoError(t, err)

	// Verify results
	assert.Len(t, tools, 2)
	assert.Equal(t, []string{"1.13.4"}, tools["hashicorp/terraform"])
	assert.Equal(t, []string{"1.34.1"}, tools["kubernetes/kubectl"])
}

func TestLockFileManager_Verify(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Add tool to create valid lock file
	err := mgr.AddTool(
		ctx, "hashicorp/terraform", "1.13.4",
		WithURL("https://example.com/terraform.zip"),
		WithChecksum("sha256:abc123"),
	)
	require.NoError(t, err)

	// Verify
	err = mgr.Verify(ctx)
	assert.NoError(t, err)
}

func TestLockFileManager_Name(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	mgr := NewLockFileManager(config)

	assert.Equal(t, "lockfile", mgr.Name())
}

func TestLockFileManager_DefaultFilePath(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: ".custom-tools",
			UseLockFile: true,
		},
	}

	mgr := NewLockFileManager(config)

	// Should use install_path/toolchain.lock.yaml
	expectedPath := filepath.Join(".custom-tools", "toolchain.lock.yaml")
	assert.Equal(t, expectedPath, mgr.filePath)
}

func TestLockFileManager_RemoveTool_Disabled(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: false,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// RemoveTool should return nil when disabled.
	err := mgr.RemoveTool(ctx, "hashicorp/terraform", "1.13.4")
	assert.NoError(t, err)
}

func TestLockFileManager_RemoveTool_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "nonexistent.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// RemoveTool should return nil when file doesn't exist.
	err := mgr.RemoveTool(ctx, "hashicorp/terraform", "1.13.4")
	assert.NoError(t, err)
}

func TestLockFileManager_RemoveTool_ToolNotInLockfile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Add a tool first.
	err := mgr.AddTool(ctx, "hashicorp/terraform", "1.13.4")
	require.NoError(t, err)

	// Try to remove a tool that doesn't exist.
	err = mgr.RemoveTool(ctx, "kubernetes/kubectl", "1.34.1")
	assert.NoError(t, err)

	// Original tool should still exist.
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "hashicorp/terraform")
}

// TestLockFileManager_RemoveTool_NullToolEntryReturnsErrorInsteadOfPanicking reproduces a panic
// risk: a hand-edited or corrupted toolchain.lock.yaml can have an explicit YAML null under an
// existing tool key (the map key is present in lock.Tools, but the value is a nil *Tool).
// RemoveTool used to dereference existingTool.Versions unconditionally once the key existed,
// which panics on a nil pointer dereference when removing a specific (non-empty) version.
func TestLockFileManager_RemoveTool_NullToolEntryReturnsErrorInsteadOfPanicking(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	content := `version: 1
tools:
  hashicorp/terraform:
metadata:
  lock_file_version: 2
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o644))

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	var err error
	require.NotPanics(t, func() {
		err = mgr.RemoveTool(ctx, "hashicorp/terraform", "1.13.4")
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, lockfile.ErrToolEntryNil)
}

func TestLockFileManager_RemoveTool_VersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Add a tool with specific version.
	err := mgr.AddTool(ctx, "hashicorp/terraform", "1.13.4")
	require.NoError(t, err)

	// Try to remove with wrong version.
	err = mgr.RemoveTool(ctx, "hashicorp/terraform", "1.10.0")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLockfileVersionMismatch)
}

// TestLockFileManager_RemoveTool_VersionMismatchListsVersionsSorted verifies the
// version-mismatch error's lockfile_versions context is sorted, not raw Go map iteration
// order. Map iteration order is randomized, so an unsorted join meant the exact same failure
// (removing a version that isn't locked) could render different error text on different runs
// -- GetTools already calls sort.Strings for this same reason.
func TestLockFileManager_RemoveTool_VersionMismatchListsVersionsSorted(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	for _, v := range []string{"1.13.4", "1.9.8", "1.11.0"} {
		require.NoError(t, mgr.AddTool(ctx, "hashicorp/terraform", v))
	}

	err := mgr.RemoveTool(ctx, "hashicorp/terraform", "9.9.9")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLockfileVersionMismatch)

	// errUtils.GetContext/HasContext parse safe details as space-separated "key=value" pairs,
	// so they can't recover a value that itself contains spaces (like a ", "-joined list) --
	// check the raw safe-detail text directly instead.
	found := false
	for _, payload := range cockroachdberrors.GetAllSafeDetails(err) {
		for _, detail := range payload.SafeDetails {
			if strings.Contains(detail, "lockfile_versions=1.11.0, 1.13.4, 1.9.8") {
				found = true
			}
		}
	}
	assert.True(t, found, "expected the error's safe details to contain the sorted version list")
}

// TestLockFileManager_AddTool_RejectsEmptyVersion verifies AddTool rejects an empty version
// instead of silently creating a lock entry keyed by "". RemoveTool gives an empty version a
// specific meaning (remove every locked version); AddTool previously gave the same input no
// meaning at all, so it wrote a bogus "" key that could only ever be removed by wiping the
// whole tool. SetDefault forwards straight through to AddTool, so this guard covers it too.
func TestLockFileManager_AddTool_RejectsEmptyVersion(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	err := mgr.AddTool(ctx, "hashicorp/terraform", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLockfileEmptyVersion)

	// No lock file should have been written at all -- the guard must fire before any load/save.
	_, statErr := os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(statErr), "AddTool must not write a lock file when rejecting an empty version")

	// SetDefault forwards to AddTool, so it must reject the same input the same way.
	err = mgr.SetDefault(ctx, "hashicorp/terraform", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrLockfileEmptyVersion)
}

func TestLockFileManager_RemoveTool_EmptyVersion(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Add a tool.
	err := mgr.AddTool(ctx, "hashicorp/terraform", "1.13.4")
	require.NoError(t, err)

	// Remove with empty version (should remove regardless of version).
	err = mgr.RemoveTool(ctx, "hashicorp/terraform", "")
	assert.NoError(t, err)

	// Tool should be removed.
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "hashicorp/terraform")
}

func TestLockFileManager_SetDefault_Disabled(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: false,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// SetDefault should return nil when disabled.
	err := mgr.SetDefault(ctx, "hashicorp/terraform", "1.13.4")
	assert.NoError(t, err)
}

func TestLockFileManager_GetTools_Disabled(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: false,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// GetTools should return nil when disabled.
	tools, err := mgr.GetTools(ctx)
	assert.NoError(t, err)
	assert.Nil(t, tools)
}

func TestLockFileManager_Verify_Disabled(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: false,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Verify should return nil when disabled.
	err := mgr.Verify(ctx)
	assert.NoError(t, err)
}

func TestLockFileManager_DefaultFilePath_NoInstallPath(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			// No InstallPath set.
		},
	}

	mgr := NewLockFileManager(config)

	// Should use default .tools/toolchain.lock.yaml.
	expectedPath := filepath.Join(".tools", "toolchain.lock.yaml")
	assert.Equal(t, expectedPath, mgr.filePath)
}

func TestLockFileManager_AddTool_WithPlatform(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "toolchain.lock.yaml")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseLockFile: true,
			LockFile:    tmpFile,
		},
	}

	mgr := NewLockFileManager(config)
	ctx := context.Background()

	// Add tool with explicit platform.
	err := mgr.AddTool(
		ctx, "hashicorp/terraform", "1.13.4",
		WithPlatform("linux_amd64"),
		WithURL("https://example.com/terraform_linux.zip"),
		WithChecksum("sha256:linux123"),
	)
	require.NoError(t, err)

	// Verify file contains platform-specific info.
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "linux_amd64")
}
