package client

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
