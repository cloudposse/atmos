package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// sep is the OS-specific PATH list separator (":" on Unix, ";" on Windows).
var sep = string(os.PathListSeparator)

func TestBuildMCPJSONEntry_NoAuth(t *testing.T) {
	cfg := &schema.MCPServerConfig{
		Command: "uvx",
		Args:    []string{"awslabs.aws-docs@latest"},
		Env:     map[string]string{"FASTMCP_LOG_LEVEL": "ERROR"},
	}
	entry := BuildMCPJSONEntry(cfg, "")
	assert.Equal(t, "uvx", entry.Command)
	assert.Equal(t, []string{"awslabs.aws-docs@latest"}, entry.Args)
	assert.Equal(t, "ERROR", entry.Env["FASTMCP_LOG_LEVEL"])
}

func TestBuildMCPJSONEntry_WithAuth(t *testing.T) {
	cfg := &schema.MCPServerConfig{
		Command:  "uvx",
		Args:     []string{"awslabs.billing@latest"},
		Env:      map[string]string{"AWS_REGION": "us-east-1"},
		Identity: "readonly",
	}
	entry := BuildMCPJSONEntry(cfg, "")
	assert.Equal(t, "atmos", entry.Command)
	assert.Equal(t, []string{"auth", "exec", "-i", "readonly", "--", "uvx", "awslabs.billing@latest"}, entry.Args)
	assert.Equal(t, "us-east-1", entry.Env["AWS_REGION"])
}

func TestBuildMCPJSONEntry_WithToolchainPATH(t *testing.T) {
	cfg := &schema.MCPServerConfig{
		Command: "uvx",
		Args:    []string{"server@latest"},
		Env:     map[string]string{"KEY": "val"},
	}
	entry := BuildMCPJSONEntry(cfg, "/toolchain/bin")
	assert.Contains(t, entry.Env["PATH"], "/toolchain/bin")
}

func TestBuildMCPJSONEntry_ToolchainPATH_PrependedToExisting(t *testing.T) {
	cfg := &schema.MCPServerConfig{
		Command: "uvx",
		Args:    []string{"server@latest"},
		Env:     map[string]string{"PATH": "/usr/bin"},
	}
	entry := BuildMCPJSONEntry(cfg, "/toolchain/bin")
	assert.True(t, strings.HasPrefix(entry.Env["PATH"], "/toolchain/bin"))
	assert.Contains(t, entry.Env["PATH"], "/usr/bin")
}

func TestBuildMCPJSONEntry_DoesNotMutateOriginal(t *testing.T) {
	originalEnv := map[string]string{"KEY": "val"}
	cfg := &schema.MCPServerConfig{
		Command: "uvx",
		Args:    []string{"server@latest"},
		Env:     originalEnv,
	}
	entry := BuildMCPJSONEntry(cfg, "/toolchain/bin")
	// Original env should not have PATH injected.
	_, hasPATH := originalEnv["PATH"]
	assert.False(t, hasPATH, "original env should not be mutated")
	// But the entry should have it.
	assert.Contains(t, entry.Env["PATH"], "/toolchain/bin")
}

func TestGenerateMCPConfig(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{
		"aws-docs": {Command: "uvx", Args: []string{"docs@latest"}},
		"aws-iam":  {Command: "uvx", Args: []string{"iam@latest"}, Identity: "admin"},
	}
	config := GenerateMCPConfig(servers, "")
	assert.Len(t, config.MCPServers, 2)
	assert.Equal(t, "uvx", config.MCPServers["aws-docs"].Command)
	assert.Equal(t, "atmos", config.MCPServers["aws-iam"].Command) // Wrapped with auth.
}

func TestGenerateMCPConfig_EmptyServers(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{}
	config := GenerateMCPConfig(servers, "")
	assert.NotNil(t, config.MCPServers)
	assert.Empty(t, config.MCPServers)
}

func TestWriteMCPConfigToTempFile(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{
		"test-server": {Command: "echo", Args: []string{"hello"}},
	}
	tmpFile, err := WriteMCPConfigToTempFile(servers, "")
	require.NoError(t, err)
	defer os.Remove(tmpFile)

	// Read and parse the file.
	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)

	var config MCPJSONConfig
	require.NoError(t, json.Unmarshal(data, &config))
	assert.Len(t, config.MCPServers, 1)
	assert.Equal(t, "echo", config.MCPServers["test-server"].Command)

	// Check file permissions (skip on Windows — no Unix-style permissions).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(tmpFile)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

// TestWriteMCPConfigToTempFile_ConcurrentWritesGetDistinctPaths is the
// regression guard for issue #4 in docs/fixes/2026-05-15-mcp-review-fixes.md.
//
// Pre-fix, WriteMCPConfigToTempFile used a fixed path
// (os.TempDir()/atmos-mcp-config.json) and two concurrent invocations
// raced on the same file — the slower writer's content silently
// overwrote the faster's, and a slower reader could see partial JSON.
// Post-fix the function uses os.CreateTemp with the pattern
// "atmos-mcp-config-*.json", so each invocation gets a unique path.
//
// The test drives 16 concurrent writes (each with a distinct, identifiable
// server name) and asserts:
//
//  1. All 16 writes succeed.
//  2. All 16 returned paths are pairwise distinct.
//  3. Each file on disk contains the server name the goroutine wrote —
//     proving no cross-talk (i.e., no writer clobbered another's content).
func TestWriteMCPConfigToTempFile_ConcurrentWritesGetDistinctPaths(t *testing.T) {
	const goroutines = 16

	// Pre-size the result slices and write by goroutine index so the
	// assertion-side code can correlate path[i] with "server-i". An
	// append-based approach loses that mapping because goroutines finish
	// in arbitrary order — leading to false "content was clobbered"
	// failures even when nothing was actually clobbered.
	var (
		wg    sync.WaitGroup
		paths = make([]string, goroutines)
		errs  = make([]error, goroutines)
	)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			serverName := fmt.Sprintf("server-%d", idx)
			servers := map[string]schema.MCPServerConfig{
				serverName: {Command: "echo", Args: []string{fmt.Sprintf("%d", idx)}},
			}

			path, err := WriteMCPConfigToTempFile(servers, "")
			// Each goroutine writes to its own slot — no mutex needed,
			// and -race stays clean.
			paths[idx] = path
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	// Cleanup every successfully-written file at the end.
	t.Cleanup(func() {
		for _, p := range paths {
			if p != "" {
				_ = os.Remove(p)
			}
		}
	})

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d failed to write", i)
	}

	// All paths must be distinct — the headline assertion.
	seen := make(map[string]int, len(paths))
	for i, p := range paths {
		if prev, ok := seen[p]; ok {
			t.Fatalf("concurrent writes returned the same path twice: goroutines %d and %d both wrote %q", prev, i, p)
		}
		seen[p] = i
	}

	// Every file must contain the server name its goroutine wrote — proves
	// no goroutine's content was clobbered by another.
	for i, p := range paths {
		data, err := os.ReadFile(p)
		require.NoError(t, err, "could not read file from goroutine %d", i)

		var config MCPJSONConfig
		require.NoError(t, json.Unmarshal(data, &config), "file from goroutine %d is not valid JSON: %s", i, string(data))

		wantServer := fmt.Sprintf("server-%d", i)
		require.Contains(t, config.MCPServers, wantServer,
			"file from goroutine %d (path=%s) is missing its own server %q — content was clobbered by another writer",
			i, p, wantServer)
	}
}

func TestCopyEnv(t *testing.T) {
	original := map[string]string{"A": "1", "B": "2"}
	copied := copyEnv(original)
	assert.Equal(t, original, copied)
	// Mutating copy should not affect original.
	copied["C"] = "3"
	_, hasC := original["C"]
	assert.False(t, hasC)
}

func TestCopyEnv_UppercasesKeys(t *testing.T) {
	// Simulates Viper-lowercased env keys being restored.
	lowercased := map[string]string{
		"aws_region":           "us-east-1",
		"fastmcp_log_level":    "ERROR",
		"read_operations_only": "true",
	}
	result := copyEnv(lowercased)
	assert.Equal(t, "us-east-1", result["AWS_REGION"])
	assert.Equal(t, "ERROR", result["FASTMCP_LOG_LEVEL"])
	assert.Equal(t, "true", result["READ_OPERATIONS_ONLY"])
	// Original lowercase keys should not exist.
	_, hasLower := result["aws_region"]
	assert.False(t, hasLower)
}

func TestCopyEnv_Nil(t *testing.T) {
	result := copyEnv(nil)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// TestWriteMCPConfigToTempFile_CreateTempFailure exercises the
// os.CreateTemp failure branch in WriteMCPConfigToTempFile by pointing
// TMPDIR at a path that doesn't exist. Without this test the CreateTemp
// error wrap is unreachable from the existing happy-path / concurrent
// tests, leaving the wrapped sentinel (errUtils.ErrMCPConfigWriteFailed)
// untested.
func TestWriteMCPConfigToTempFile_CreateTempFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		// On Windows, os.TempDir resolution differs and TMP/TEMP env
		// vars interact with the OS API in ways that aren't trivially
		// portable. The Unix path covers the contract; Windows-specific
		// failure modes are tested by the OS, not this test suite.
		t.Skip("TMPDIR override semantics differ on Windows")
	}

	// Point TMPDIR at a path that doesn't exist. os.CreateTemp uses
	// os.TempDir() which honors TMPDIR; CreateTemp will fail on the
	// underlying open(2) because the dir doesn't exist.
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "does-not-exist"))

	servers := map[string]schema.MCPServerConfig{
		"test-server": {Command: "echo", Args: []string{"hello"}},
	}
	path, err := WriteMCPConfigToTempFile(servers, "")
	require.Error(t, err,
		"CreateTemp must fail when TMPDIR points at a missing directory")
	assert.Empty(t, path,
		"failed write must return empty path so callers don't try to clean up a non-existent file")
	assert.ErrorIs(t, err, errUtils.ErrMCPConfigWriteFailed,
		"the CreateTemp failure must be wrapped with ErrMCPConfigWriteFailed for errors.Is matching")
}

// TestWriteMCPConfigToTempFile_TempDirHonored verifies the cooperative
// contract with os.TempDir — when TMPDIR is set to a writable directory,
// WriteMCPConfigToTempFile creates the temp file IN that directory (not
// the system default). This is what makes the concurrent test work in
// CI runners and matters for users who set TMPDIR to redirect writes
// onto a faster volume.
func TestWriteMCPConfigToTempFile_TempDirHonored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TMPDIR override semantics differ on Windows")
	}

	overrideDir := t.TempDir()
	t.Setenv("TMPDIR", overrideDir)

	servers := map[string]schema.MCPServerConfig{
		"test-server": {Command: "echo", Args: []string{"hello"}},
	}
	path, err := WriteMCPConfigToTempFile(servers, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })

	// Resolve symlinks for both — macOS tends to symlink /tmp → /private/tmp
	// and t.Setenv("TMPDIR", ...) sometimes lands either side of that link
	// depending on whether t.TempDir() already resolved it. Resolving both
	// sides makes the assertion robust to that platform quirk.
	resolvedOverride, err := filepath.EvalSymlinks(overrideDir)
	require.NoError(t, err)
	resolvedPath, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(resolvedPath, resolvedOverride),
		"WriteMCPConfigToTempFile must respect TMPDIR; got path %q (resolved %q), expected prefix %q (resolved %q)",
		path, resolvedPath, overrideDir, resolvedOverride)
}

func TestDeduplicatePATH(t *testing.T) {
	// Use os.PathListSeparator-aware paths for cross-platform compatibility.
	// On Windows, PATH uses ";" as separator; on Unix, ":".
	join := func(parts ...string) string {
		return strings.Join(parts, sep)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no duplicates",
			input:    join("/usr/bin", "/usr/local/bin", "/opt/bin"),
			expected: join("/usr/bin", "/usr/local/bin", "/opt/bin"),
		},
		{
			name:     "duplicates removed",
			input:    join("/toolchain/bin", "/usr/bin", "/toolchain/bin", "/usr/bin"),
			expected: join("/toolchain/bin", "/usr/bin"),
		},
		{
			name:     "empty entries removed",
			input:    "/usr/bin" + sep + sep + "/usr/local/bin" + sep,
			expected: join("/usr/bin", "/usr/local/bin"),
		},
		{
			name:     "preserves order",
			input:    join("/cc", "/aa", "/bb", "/aa", "/cc"),
			expected: join("/cc", "/aa", "/bb"),
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicatePATH(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInjectToolchainPATH_Deduplicates(t *testing.T) {
	existingPATH := strings.Join([]string{"/usr/bin", "/usr/local/bin"}, sep)
	toolchainPATH := strings.Join([]string{"/toolchain/bin", "/usr/bin"}, sep)

	env := map[string]string{
		"PATH": existingPATH,
	}
	// Toolchain PATH includes a dir already in the existing PATH.
	injectToolchainPATH(env, toolchainPATH)
	path := env["PATH"]
	// /usr/bin should appear only once.
	count := strings.Count(path, "/usr/bin")
	assert.Equal(t, 1, count, "PATH should not contain duplicate /usr/bin entries")
	// Toolchain should be first.
	assert.True(t, strings.HasPrefix(path, "/toolchain/bin"))
}
