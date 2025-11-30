package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
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
	err := mgr.AddTool(ctx, "hashicorp/terraform", "1.13.4",
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

func TestLockFileManager_AddTool_UpdateExisting(t *testing.T) {
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

	// Add initial version
	err := mgr.AddTool(ctx, "hashicorp/terraform", "1.10.0",
		WithURL("https://example.com/terraform-1.10.0.zip"),
		WithChecksum("sha256:old123"),
	)
	require.NoError(t, err)

	// Update to new version
	err = mgr.AddTool(ctx, "hashicorp/terraform", "1.13.4",
		WithURL("https://example.com/terraform-1.13.4.zip"),
		WithChecksum("sha256:new123"),
	)
	require.NoError(t, err)

	// Verify file contains new version
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "1.13.4")
	assert.Contains(t, string(content), "sha256:new123")
	assert.NotContains(t, string(content), "1.10.0")
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
	err := mgr.AddTool(ctx, "hashicorp/terraform", "1.13.4",
		WithURL("https://example.com/terraform.zip"),
		WithChecksum("sha256:abc123"),
	)
	require.NoError(t, err)

	err = mgr.AddTool(ctx, "kubernetes/kubectl", "1.34.1",
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
	err := mgr.AddTool(ctx, "hashicorp/terraform", "1.13.4",
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
