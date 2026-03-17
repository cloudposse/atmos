package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitCliConfig_ExplicitBasePath_ResolvesRelativeToCWD verifies that when AtmosBasePath
// is explicitly provided (e.g., via --base-path or ATMOS_BASE_PATH), it resolves relative
// to the current working directory, not the git root.
func TestInitCliConfig_ExplicitBasePath_ResolvesRelativeToCWD(t *testing.T) {
	// Create a temp directory to simulate a project layout.
	tmpDir := t.TempDir()

	// Resolve symlinks (macOS /var -> /private/var).
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Create a subdirectory to simulate CWD being different from project root.
	subDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	// Create a relative base path target.
	relBasePath := filepath.Join(".terraform", "modules", "monorepo")
	absTarget := filepath.Join(subDir, relBasePath)
	require.NoError(t, os.MkdirAll(absTarget, 0o755))

	// Create minimal atmos.yaml in the target.
	atmosYaml := filepath.Join(absTarget, "atmos.yaml")
	require.NoError(t, os.WriteFile(atmosYaml, []byte("base_path: ./\nstacks:\n  base_path: stacks\n"), 0o644))

	// Create stacks directory so config loading doesn't fail.
	require.NoError(t, os.MkdirAll(filepath.Join(absTarget, "stacks"), 0o755))

	// Change to the subdirectory (simulating terraform-provider-utils context).
	t.Chdir(subDir)

	// Provide AtmosBasePath as a relative path (what terraform-provider-utils does).
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath: relBasePath,
	}

	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)

	// The base path should be absolute and resolve to CWD + relBasePath.
	assert.True(t, filepath.IsAbs(atmosConfig.BasePath),
		"BasePath should be absolute, got: %s", atmosConfig.BasePath)
	assert.Equal(t, absTarget, atmosConfig.BasePath,
		"BasePath should resolve relative to CWD, not git root")
}

// TestInitCliConfig_ExplicitBasePath_AbsolutePassedThrough verifies that an absolute
// AtmosBasePath is passed through without modification.
func TestInitCliConfig_ExplicitBasePath_AbsolutePassedThrough(t *testing.T) {
	tmpDir := t.TempDir()

	// Resolve symlinks (macOS /var -> /private/var).
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Create minimal atmos.yaml.
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "atmos.yaml"),
		[]byte("base_path: ./\nstacks:\n  base_path: stacks\n"),
		0o644,
	))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "stacks"), 0o755))

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath: tmpDir,
	}

	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)

	assert.Equal(t, tmpDir, atmosConfig.BasePath,
		"Absolute AtmosBasePath should be used as-is")
}

// TestInitCliConfig_EmptyBasePath_DefaultsToAbsolute verifies that when AtmosBasePath is empty,
// the default resolution produces an absolute path.
func TestInitCliConfig_EmptyBasePath_DefaultsToAbsolute(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath: "",
	}

	// InitCliConfig with processStacks=false — BasePath gets populated by AtmosConfigAbsolutePaths.
	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)

	// After AtmosConfigAbsolutePaths, BasePath should be absolute (from git root, config dir, or CWD).
	if atmosConfig.BasePath != "" {
		assert.True(t, filepath.IsAbs(atmosConfig.BasePath),
			"Default BasePath should be absolute, got: %s", atmosConfig.BasePath)
	}
}

// TestInitCliConfig_EnvVarBasePath_ResolvesRelativeToCWD verifies that when ATMOS_BASE_PATH
// is set as an environment variable with a relative path, it resolves relative to CWD,
// not git root. This is Tyler Rankin's exact scenario: ATMOS_BASE_PATH=.terraform/modules/monorepo
// set on a Spacelift worker where CWD is a component directory.
func TestInitCliConfig_EnvVarBasePath_ResolvesRelativeToCWD(t *testing.T) {
	tmpDir := t.TempDir()

	// Resolve symlinks (macOS /var -> /private/var).
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Create a subdirectory to simulate CWD being a component directory.
	subDir := filepath.Join(tmpDir, "components", "terraform", "iam-delegated-roles")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	// Create the relative base path target (simulates .terraform/modules/monorepo).
	relBasePath := filepath.Join(".terraform", "modules", "monorepo")
	absTarget := filepath.Join(subDir, relBasePath)
	require.NoError(t, os.MkdirAll(absTarget, 0o755))

	// Create minimal atmos.yaml in the target.
	require.NoError(t, os.WriteFile(
		filepath.Join(absTarget, "atmos.yaml"),
		[]byte("base_path: ./\nstacks:\n  base_path: stacks\n"),
		0o644,
	))
	require.NoError(t, os.MkdirAll(filepath.Join(absTarget, "stacks"), 0o755))

	// Change to the component directory.
	t.Chdir(subDir)

	// Set ATMOS_BASE_PATH as env var (what Tyler does on Spacelift).
	t.Setenv("ATMOS_BASE_PATH", relBasePath)

	// No AtmosBasePath in struct — the env var is the source.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)

	// After AtmosConfigAbsolutePaths, BasePathAbsolute should resolve to CWD + relBasePath.
	assert.True(t, filepath.IsAbs(atmosConfig.BasePathAbsolute),
		"BasePathAbsolute should be absolute, got: %s", atmosConfig.BasePathAbsolute)
	assert.Equal(t, absTarget, atmosConfig.BasePathAbsolute,
		"ATMOS_BASE_PATH env var should resolve relative to CWD, not git root")
}

// TestFindAllStackConfigsInPathsForStack_ErrorWrapping verifies that when GetGlobMatches
// fails, the error is wrapped with the ErrFailedToFindImport sentinel and uses the error
// builder pattern with actionable hints.
func TestFindAllStackConfigsInPathsForStack_ErrorWrapping(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		StacksBaseAbsolutePath: filepath.Join(os.TempDir(), "nonexistent-stacks-dir-test"),
	}

	// Use a pattern that points to a nonexistent directory.
	includeStackPaths := []string{
		filepath.Join(os.TempDir(), "nonexistent-stacks-dir-test", "**", "*.yaml"),
	}

	_, _, _, err := FindAllStackConfigsInPathsForStack(
		atmosConfig,
		"test-stack",
		includeStackPaths,
		nil,
	)

	require.Error(t, err)

	// The error should wrap ErrFailedToFindImport from GetGlobMatches.
	assert.True(t, errors.Is(err, errUtils.ErrFailedToFindImport),
		"Error should wrap ErrFailedToFindImport, got: %v", err)
}

// TestFindAllStackConfigsInPaths_ErrorWrapping verifies error wrapping in the non-stack variant.
func TestFindAllStackConfigsInPaths_ErrorWrapping(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		StacksBaseAbsolutePath: filepath.Join(os.TempDir(), "nonexistent-stacks-dir-test2"),
	}

	includeStackPaths := []string{
		filepath.Join(os.TempDir(), "nonexistent-stacks-dir-test2", "**", "*.yaml"),
	}

	_, _, err := FindAllStackConfigsInPaths(
		&atmosConfig,
		includeStackPaths,
		nil,
	)

	require.Error(t, err)

	// The error should wrap ErrFailedToFindImport.
	assert.True(t, errors.Is(err, errUtils.ErrFailedToFindImport),
		"Error should wrap ErrFailedToFindImport, got: %v", err)
}
