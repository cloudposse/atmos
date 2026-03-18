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

// TestInitCliConfig_ExplicitBasePath_DotSlash_ResolvesRelativeToCWD verifies that when
// AtmosBasePath uses a dot-slash prefix (e.g., "./.terraform/modules/monorepo"), it resolves
// relative to CWD. This is Tyler's scenario: the dot-slash explicitly anchors to CWD.
func TestInitCliConfig_ExplicitBasePath_DotSlash_ResolvesRelativeToCWD(t *testing.T) {
	// Create a temp directory to simulate a project layout.
	tmpDir := t.TempDir()

	// Resolve symlinks (macOS /var -> /private/var).
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Create a subdirectory to simulate CWD being different from project root.
	subDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	// Create a relative base path target with dot-slash prefix.
	relBasePath := filepath.Join(".", ".terraform", "modules", "monorepo")
	absTarget := filepath.Join(subDir, ".terraform", "modules", "monorepo")
	require.NoError(t, os.MkdirAll(absTarget, 0o755))

	// Create minimal atmos.yaml in the target.
	atmosYaml := filepath.Join(absTarget, "atmos.yaml")
	require.NoError(t, os.WriteFile(atmosYaml, []byte("base_path: ./\nstacks:\n  base_path: stacks\n"), 0o644))

	// Create stacks directory so config loading doesn't fail.
	require.NoError(t, os.MkdirAll(filepath.Join(absTarget, "stacks"), 0o755))

	// Change to the subdirectory (simulating terraform-provider-utils context).
	t.Chdir(subDir)

	// Provide AtmosBasePath with dot-slash prefix — this is a runtime source.
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath: relBasePath,
	}

	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)

	// The base path should resolve to CWD + relative path (dot-slash = CWD anchor).
	assert.True(t, filepath.IsAbs(atmosConfig.BasePathAbsolute),
		"BasePathAbsolute should be absolute, got: %s", atmosConfig.BasePathAbsolute)
	assert.Equal(t, absTarget, atmosConfig.BasePathAbsolute,
		"Dot-slash AtmosBasePath should resolve relative to CWD, not config dir")
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

// TestInitCliConfig_EnvVarBasePath_DotSlash_ResolvesRelativeToCWD verifies that when
// ATMOS_BASE_PATH is set with a dot-slash prefix, it resolves relative to CWD.
// This is Tyler's scenario: ATMOS_BASE_PATH=./.terraform/modules/monorepo on Spacelift.
func TestInitCliConfig_EnvVarBasePath_DotSlash_ResolvesRelativeToCWD(t *testing.T) {
	tmpDir := t.TempDir()

	// Resolve symlinks (macOS /var -> /private/var).
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Create a subdirectory to simulate CWD being a component directory.
	subDir := filepath.Join(tmpDir, "components", "terraform", "iam-delegated-roles")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	// Create the relative base path target with dot-slash prefix.
	relBasePath := filepath.Join(".", ".terraform", "modules", "monorepo")
	absTarget := filepath.Join(subDir, ".terraform", "modules", "monorepo")
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

	// Set ATMOS_BASE_PATH with dot-slash prefix (Tyler's fix).
	// Resolves to CWD (not config dir). In shell context, "." means "here" is CWD.
	t.Setenv("ATMOS_BASE_PATH", relBasePath)

	// No AtmosBasePath in struct — the env var is the source.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)

	// After AtmosConfigAbsolutePaths, BasePathAbsolute should resolve to CWD + relBasePath.
	assert.True(t, filepath.IsAbs(atmosConfig.BasePathAbsolute),
		"BasePathAbsolute should be absolute, got: %s", atmosConfig.BasePathAbsolute)
	assert.Equal(t, absTarget, atmosConfig.BasePathAbsolute,
		"ATMOS_BASE_PATH env var with dot-slash should resolve relative to CWD")
}

// TestInitCliConfig_EnvVarBasePath_Dot_ResolvesToCWD verifies that ATMOS_BASE_PATH=.
// Resolves to CWD (not config dir). In shell context, "." means "here" is CWD.
func TestInitCliConfig_EnvVarBasePath_Dot_ResolvesToCWD(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Create project layout: config in one dir, CWD in another.
	configDir := filepath.Join(tmpDir, "config")
	cwdDir := filepath.Join(tmpDir, "workdir")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.MkdirAll(cwdDir, 0o755))

	// Create atmos.yaml in configDir.
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "atmos.yaml"),
		[]byte("base_path: ./\nstacks:\n  base_path: stacks\n"),
		0o644,
	))
	require.NoError(t, os.MkdirAll(filepath.Join(cwdDir, "stacks"), 0o755))

	t.Chdir(cwdDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", configDir)
	t.Setenv("ATMOS_BASE_PATH", ".")
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")

	atmosConfig, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// ATMOS_BASE_PATH=. in shell context should resolve to CWD, not config dir.
	assert.Equal(t, cwdDir, atmosConfig.BasePathAbsolute,
		"ATMOS_BASE_PATH=. should resolve to CWD (shell convention), not config dir")
}

// TestResolveAbsolutePath_DotPrefix_SourceAware verifies that dot-prefixed paths
// resolve differently based on source: config → config dir, runtime → CWD.
func TestResolveAbsolutePath_DotPrefix_SourceAware(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	configDir := filepath.Join(tmpDir, "config")
	cwdDir := filepath.Join(tmpDir, "workdir")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.MkdirAll(cwdDir, 0o755))

	t.Chdir(cwdDir)

	tests := []struct {
		name     string
		path     string
		source   string
		expected string
	}{
		{
			name:     "dot from config resolves to config dir",
			path:     ".",
			source:   "",
			expected: configDir,
		},
		{
			name:     "dot from runtime resolves to CWD",
			path:     ".",
			source:   "runtime",
			expected: cwdDir,
		},
		{
			name:     "dot-slash-foo from config resolves to config dir",
			path:     "./foo",
			source:   "",
			expected: filepath.Join(configDir, "foo"),
		},
		{
			name:     "dot-slash-foo from runtime resolves to CWD",
			path:     "./foo",
			source:   "runtime",
			expected: filepath.Join(cwdDir, "foo"),
		},
		{
			name:     "dot-dot from config resolves relative to config dir",
			path:     "..",
			source:   "",
			expected: tmpDir,
		},
		{
			name:     "dot-dot from runtime resolves relative to CWD",
			path:     "..",
			source:   "runtime",
			expected: tmpDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveAbsolutePath(tt.path, configDir, tt.source)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveAbsolutePath_BarePath_SourceIndependent verifies that bare paths
// (no dot prefix) go through the same git root search regardless of source.
func TestResolveAbsolutePath_BarePath_SourceIndependent(t *testing.T) {
	gitRoot := getGitRootOrEmpty()
	require.NotEmpty(t, gitRoot, "test requires git root discovery")

	// "go.mod" exists at git root — bare path should find it regardless of source.
	_, err := os.Stat(filepath.Join(gitRoot, "go.mod"))
	require.NoError(t, err)

	t.Chdir(filepath.Join(gitRoot, "pkg", "config"))

	configResult, err := resolveAbsolutePath("go.mod", "", "")
	require.NoError(t, err)

	runtimeResult, err := resolveAbsolutePath("go.mod", "", "runtime")
	require.NoError(t, err)

	// Both should resolve to git root — bare paths are source-independent.
	assert.Equal(t, filepath.Join(gitRoot, "go.mod"), configResult,
		"bare path from config should resolve via git root")
	assert.Equal(t, filepath.Join(gitRoot, "go.mod"), runtimeResult,
		"bare path from runtime should resolve via git root (same as config)")
	assert.Equal(t, configResult, runtimeResult,
		"bare paths must resolve identically regardless of source")
}

// TestTryResolveWithGitRoot_ExistingPathAtGitRoot verifies that when a simple relative path
// exists at the git root, resolveAbsolutePath returns the git-root-joined path.
func TestTryResolveWithGitRoot_ExistingPathAtGitRoot(t *testing.T) {
	gitRoot := getGitRootOrEmpty()
	require.NotEmpty(t, gitRoot, "test requires git root discovery")

	pathAtGitRoot := "go.mod"
	_, err := os.Stat(filepath.Join(gitRoot, pathAtGitRoot))
	require.NoError(t, err)

	t.Chdir(filepath.Join(gitRoot, "pkg", "config"))

	resolved, err := resolveAbsolutePath(pathAtGitRoot, "", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(gitRoot, pathAtGitRoot), resolved)
}

// TestTryResolveWithGitRoot_CWDFallback verifies that when a simple relative path does NOT
// exist at the git root but DOES exist relative to CWD, the resolver falls back to the
// CWD-relative path.
func TestTryResolveWithGitRoot_CWDFallback(t *testing.T) {
	gitRoot := getGitRootOrEmpty()
	require.NotEmpty(t, gitRoot, "test requires git root discovery")

	cwdDir := filepath.Join(gitRoot, "pkg", "config", "testdata-cwd-fallback")
	require.NoError(t, os.MkdirAll(cwdDir, 0o755))
	t.Cleanup(func() { os.RemoveAll(cwdDir) })

	cwdOnlyPath := "test-cwd-fallback-unique-dir-12345"
	absExpected := filepath.Join(cwdDir, cwdOnlyPath)
	require.NoError(t, os.MkdirAll(absExpected, 0o755))
	t.Cleanup(func() { os.RemoveAll(absExpected) })

	_, statErr := os.Stat(filepath.Join(gitRoot, cwdOnlyPath))
	require.True(t, os.IsNotExist(statErr), "path should not exist at git root")

	t.Chdir(cwdDir)

	resolved, err := resolveAbsolutePath(cwdOnlyPath, "", "")
	require.NoError(t, err)
	assert.Equal(t, absExpected, resolved,
		"should fall back to CWD-relative path when git root path doesn't exist")
}

// TestTryResolveWithGitRoot_NeitherExists verifies that when a simple relative path exists
// at neither git root nor CWD, the resolver returns the git-root-joined path.
func TestTryResolveWithGitRoot_NeitherExists(t *testing.T) {
	gitRoot := getGitRootOrEmpty()
	require.NotEmpty(t, gitRoot, "test requires git root discovery")

	nonexistentPath := "nonexistent-path-that-should-not-exist-anywhere-12345"

	resolved, err := resolveAbsolutePath(nonexistentPath, "", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(gitRoot, nonexistentPath), resolved,
		"should return git-root-joined path when neither location exists")
}

// TestResolveDotPrefixPath_NoConfigPath_FallsToCWD verifies that when source is config
// but no cliConfigPath is provided, dot-prefixed paths fall back to CWD.
func TestResolveDotPrefixPath_NoConfigPath_FallsToCWD(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	cwdDir := filepath.Join(tmpDir, "workdir")
	require.NoError(t, os.MkdirAll(cwdDir, 0o755))
	t.Chdir(cwdDir)

	// Config source with empty cliConfigPath — should fall back to CWD.
	result, err := resolveAbsolutePath(".", "", "")
	require.NoError(t, err)
	assert.Equal(t, cwdDir, result,
		"dot from config with no cliConfigPath should fall back to CWD")

	// Same for dot-slash-foo.
	result, err = resolveAbsolutePath("./sub", "", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cwdDir, "sub"), result,
		"dot-slash from config with no cliConfigPath should fall back to CWD")
}

// TestResolveDotPrefixPath_DotDotSlash_Runtime verifies "../foo" from runtime resolves to CWD.
func TestResolveDotPrefixPath_DotDotSlash_Runtime(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	cwdDir := filepath.Join(tmpDir, "a", "b")
	require.NoError(t, os.MkdirAll(cwdDir, 0o755))
	t.Chdir(cwdDir)

	result, err := resolveAbsolutePath("../c", filepath.Join(tmpDir, "config"), "runtime")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "a", "c"), result,
		"../c from runtime should resolve relative to CWD")
}

// TestResolveAbsolutePath_AbsolutePassThrough verifies absolute paths pass through unchanged.
func TestResolveAbsolutePath_AbsolutePassThrough(t *testing.T) {
	// Use a real absolute path from the OS to avoid Windows drive-letter issues.
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	absPath := filepath.Join(tmpDir, "some", "absolute", "path")
	configPath := filepath.Join(tmpDir, "config")

	result, err := resolveAbsolutePath(absPath, configPath, "")
	require.NoError(t, err)
	assert.Equal(t, absPath, result, "absolute path should pass through unchanged")

	result, err = resolveAbsolutePath(absPath, "", "runtime")
	require.NoError(t, err)
	assert.Equal(t, absPath, result, "absolute path should pass through regardless of source")
}

// TestTryResolveWithConfigPath_AllBranches covers tryResolveWithConfigPath branches.
func TestTryResolveWithConfigPath_AllBranches(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	configDir := filepath.Join(tmpDir, "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	cwdDir := filepath.Join(tmpDir, "cwd")
	require.NoError(t, os.MkdirAll(cwdDir, 0o755))
	t.Chdir(cwdDir)

	tests := []struct {
		name       string
		path       string
		configPath string
		expected   string
	}{
		{
			name:       "empty path with config path returns config path",
			path:       "",
			configPath: configDir,
			expected:   configDir,
		},
		{
			name:       "relative path with config path joins them",
			path:       "stacks",
			configPath: configDir,
			expected:   filepath.Join(configDir, "stacks"),
		},
		{
			name:       "empty path and empty config path returns CWD",
			path:       "",
			configPath: "",
			expected:   cwdDir,
		},
		{
			name:       "relative path with empty config path resolves to CWD",
			path:       "stacks",
			configPath: "",
			expected:   filepath.Join(cwdDir, "stacks"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tryResolveWithConfigPath(tt.path, tt.configPath)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveAbsolutePath_BarePathNoGitRoot verifies that bare paths without git root
// fall back to config dir, then CWD.
func TestResolveAbsolutePath_BarePathNoGitRoot(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	configDir := filepath.Join(tmpDir, "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")

	// Bare path with config dir and no git root → config dir join.
	result, err := resolveAbsolutePath("stacks", configDir, "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "stacks"), result,
		"bare path without git root should resolve via config dir")

	// Bare path with no config dir and no git root → CWD join.
	result, err = resolveAbsolutePath("stacks", "", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "stacks"), result,
		"bare path without git root and config dir should resolve via CWD")
}

// TestInitCliConfig_BasePathSource_SetForStructField verifies that BasePathSource
// is set to "runtime" when AtmosBasePath is provided via struct field.
func TestInitCliConfig_BasePathSource_SetForStructField(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

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

	assert.Equal(t, "runtime", atmosConfig.BasePathSource,
		"BasePathSource should be 'runtime' when AtmosBasePath is set via struct field")
}

// TestInitCliConfig_BasePathSource_SetForEnvVar verifies that BasePathSource
// is set to "runtime" when ATMOS_BASE_PATH env var is provided.
func TestInitCliConfig_BasePathSource_SetForEnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "atmos.yaml"),
		[]byte("base_path: ./\nstacks:\n  base_path: stacks\n"),
		0o644,
	))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "stacks"), 0o755))
	t.Chdir(tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	atmosConfig, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	assert.Equal(t, "runtime", atmosConfig.BasePathSource,
		"BasePathSource should be 'runtime' when ATMOS_BASE_PATH env var is set")
}

// TestFindAllStackConfigsInPathsForStack_ErrorWrapping verifies that when GetGlobMatches
// fails, the error is wrapped with the ErrFailedToFindImport sentinel.
func TestFindAllStackConfigsInPathsForStack_ErrorWrapping(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		StacksBaseAbsolutePath: filepath.Join(os.TempDir(), "nonexistent-stacks-dir-test"),
	}

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

	assert.True(t, errors.Is(err, errUtils.ErrFailedToFindImport),
		"Error should wrap ErrFailedToFindImport, got: %v", err)
}
