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

func TestToolVersionsFileManager_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected bool
	}{
		{
			name: "enabled",
			config: &schema.AtmosConfiguration{
				Toolchain: schema.Toolchain{
					UseToolVersions: true,
				},
			},
			expected: true,
		},
		{
			name: "disabled",
			config: &schema.AtmosConfiguration{
				Toolchain: schema.Toolchain{
					UseToolVersions: false,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewToolVersionsFileManager(tt.config)
			assert.Equal(t, tt.expected, mgr.Enabled())
		})
	}
}

func TestToolVersionsFileManager_AddTool(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, ".tool-versions")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseToolVersions: true,
			VersionsFile:    tmpFile,
		},
	}

	mgr := NewToolVersionsFileManager(config)
	ctx := context.Background()

	// Add tool
	err := mgr.AddTool(ctx, "terraform", "1.13.4")
	require.NoError(t, err)

	// Verify file contents
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "terraform 1.13.4")
}

func TestToolVersionsFileManager_AddTool_AsDefault(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, ".tool-versions")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseToolVersions: true,
			VersionsFile:    tmpFile,
		},
	}

	mgr := NewToolVersionsFileManager(config)
	ctx := context.Background()

	// Add first version
	err := mgr.AddTool(ctx, "terraform", "1.10.0")
	require.NoError(t, err)

	// Add second version as default
	err = mgr.AddTool(ctx, "terraform", "1.13.4", WithAsDefault())
	require.NoError(t, err)

	// Verify file contents - default should be first
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "terraform 1.13.4 1.10.0")
}

func TestToolVersionsFileManager_AddTool_Disabled(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, ".tool-versions")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseToolVersions: false,
			VersionsFile:    tmpFile,
		},
	}

	mgr := NewToolVersionsFileManager(config)
	ctx := context.Background()

	// Add tool (should be skipped)
	err := mgr.AddTool(ctx, "terraform", "1.13.4")
	require.NoError(t, err)

	// Verify file was NOT created
	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err))
}

func TestToolVersionsFileManager_RemoveTool(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, ".tool-versions")

	// Create initial file
	err := os.WriteFile(tmpFile, []byte("terraform 1.13.4\nkubectl 1.34.1\n"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseToolVersions: true,
			VersionsFile:    tmpFile,
		},
	}

	mgr := NewToolVersionsFileManager(config)
	ctx := context.Background()

	// Remove tool
	err = mgr.RemoveTool(ctx, "terraform", "1.13.4")
	require.NoError(t, err)

	// Verify file contents
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "terraform")
	assert.Contains(t, string(content), "kubectl")
}

func TestToolVersionsFileManager_SetDefault(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, ".tool-versions")

	// Create initial file with multiple versions
	err := os.WriteFile(tmpFile, []byte("terraform 1.10.0 1.13.4\n"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseToolVersions: true,
			VersionsFile:    tmpFile,
		},
	}

	mgr := NewToolVersionsFileManager(config)
	ctx := context.Background()

	// Set different version as default
	err = mgr.SetDefault(ctx, "terraform", "1.13.4")
	require.NoError(t, err)

	// Verify file contents
	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "terraform 1.13.4 1.10.0")
}

func TestToolVersionsFileManager_GetTools(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, ".tool-versions")

	// Create initial file
	err := os.WriteFile(tmpFile, []byte("terraform 1.13.4 1.10.0\nkubectl 1.34.1\n"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseToolVersions: true,
			VersionsFile:    tmpFile,
		},
	}

	mgr := NewToolVersionsFileManager(config)
	ctx := context.Background()

	// Get tools
	tools, err := mgr.GetTools(ctx)
	require.NoError(t, err)

	// Verify results
	assert.Len(t, tools, 2)
	assert.Equal(t, []string{"1.13.4", "1.10.0"}, tools["terraform"])
	assert.Equal(t, []string{"1.34.1"}, tools["kubectl"])
}

func TestToolVersionsFileManager_Verify(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, ".tool-versions")

	config := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			UseToolVersions: true,
			VersionsFile:    tmpFile,
		},
	}

	mgr := NewToolVersionsFileManager(config)
	ctx := context.Background()

	// Verify (should always succeed for .tool-versions)
	err := mgr.Verify(ctx)
	assert.NoError(t, err)
}

func TestToolVersionsFileManager_Name(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	mgr := NewToolVersionsFileManager(config)

	assert.Equal(t, "tool-versions", mgr.Name())
}
