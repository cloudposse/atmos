package toolchain

import (
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmitPath(t *testing.T) {
	setupTestIO(t)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			ToolsDir: t.TempDir(),
		},
	})
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file with some tools
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"helm":      {"3.17.4"},
		},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Temporarily set the global toolVersionsFile variable
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath, ToolsDir: tempDir}})

	// Test that runInstall with no arguments doesn't error
	// This prevents regression where the function might error when no specific tool is provided
	err = RunInstall("", false, false)
	assert.NoError(t, err)

	// Test basic functionality with export flag
	err = EmitPath(true, false, false)
	if err != nil {
		t.Errorf("EmitPath() error = %v", err)
	}
}
