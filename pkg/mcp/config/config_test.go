package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInferName(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "url with path segment", target: "https://mcp.example.com/aws-docs", want: "aws-docs"},
		{name: "url with trailing slash", target: "https://mcp.example.com/aws-docs/", want: "aws-docs"},
		{name: "url host only", target: "https://mcp.example.com", want: "mcp-example-com"},
		{name: "bare command", target: "uvx", want: "uvx"},
		{name: "command with versioned package", target: "uvx awslabs.aws-docs@latest", want: "awslabs-aws-docs"},
		{name: "npx scoped package with flags", target: "npx -y @org/mcp-server", want: "mcp-server"},
		{name: "node script path", target: "node /path/to/server.js", want: "server-js"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, InferName(tt.target))
		})
	}
}

func TestParseKeyValuePairs(t *testing.T) {
	tests := []struct {
		name    string
		pairs   []string
		want    map[string]string
		wantErr bool
	}{
		{name: "empty", pairs: nil, want: nil},
		{name: "single", pairs: []string{"KEY=value"}, want: map[string]string{"KEY": "value"}},
		{name: "value with equals", pairs: []string{"KEY=a=b"}, want: map[string]string{"KEY": "a=b"}},
		{name: "missing equals", pairs: []string{"KEYvalue"}, wantErr: true},
		{name: "empty key", pairs: []string{"=value"}, wantErr: true},
		{name: "duplicate key last wins", pairs: []string{"KEY=a", "KEY=b"}, want: map[string]string{"KEY": "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKeyValuePairs(tt.pairs)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseHeaderPairs(t *testing.T) {
	tests := []struct {
		name    string
		pairs   []string
		want    map[string]string
		wantErr bool
	}{
		{name: "empty", pairs: nil, want: nil},
		{name: "single", pairs: []string{"Authorization: Bearer x"}, want: map[string]string{"Authorization": "Bearer x"}},
		{name: "no space after colon", pairs: []string{"X-Key:value"}, want: map[string]string{"X-Key": "value"}},
		{name: "missing colon", pairs: []string{"Authorization Bearer x"}, wantErr: true},
		{name: "empty key", pairs: []string{": value"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHeaderPairs(tt.pairs)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseServerSpec(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	t.Run("stdio single token", func(t *testing.T) {
		name, cfg, err := ParseServerSpec(atmosConfig, ServerSpec{Target: "uvx"})
		require.NoError(t, err)
		assert.Equal(t, "uvx", name)
		assert.Equal(t, "uvx", cfg.Command)
		assert.Empty(t, cfg.Args)
	})

	t.Run("stdio multi token command string", func(t *testing.T) {
		name, cfg, err := ParseServerSpec(atmosConfig, ServerSpec{Target: "npx -y @org/mcp-server --flag value"})
		require.NoError(t, err)
		assert.Equal(t, "mcp-server", name)
		assert.Equal(t, "npx", cfg.Command)
		assert.Equal(t, []string{"-y", "@org/mcp-server", "--flag", "value"}, cfg.Args)
	})

	t.Run("http url", func(t *testing.T) {
		name, cfg, err := ParseServerSpec(atmosConfig, ServerSpec{Target: "https://mcp.example.com/aws-docs"})
		require.NoError(t, err)
		assert.Equal(t, "aws-docs", name)
		assert.Equal(t, schema.MCPTransportHTTP, cfg.Type)
		assert.Equal(t, "https://mcp.example.com/aws-docs", cfg.URL)
	})

	t.Run("explicit http transport on url", func(t *testing.T) {
		_, cfg, err := ParseServerSpec(atmosConfig, ServerSpec{Target: "https://mcp.example.com/mcp", Transport: "http"})
		require.NoError(t, err)
		assert.Equal(t, schema.MCPTransportHTTP, cfg.Type)
	})

	t.Run("unsupported transport rejected", func(t *testing.T) {
		_, _, err := ParseServerSpec(atmosConfig, ServerSpec{Target: "https://mcp.example.com/mcp", Transport: "sse"})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUnsupportedTransport)
	})

	t.Run("empty target errors", func(t *testing.T) {
		_, _, err := ParseServerSpec(atmosConfig, ServerSpec{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errEmptyTarget)
	})

	t.Run("name override, identity, description, timeout, autostart passthrough", func(t *testing.T) {
		name, cfg, err := ParseServerSpec(atmosConfig, ServerSpec{
			Target:      "uvx server",
			Name:        "custom",
			Description: "desc",
			Identity:    "readonly",
			Timeout:     "30s",
			Env:         []string{"K=V"},
			Headers:     []string{"H: v"},
			AutoStart:   true,
		})
		require.NoError(t, err)
		assert.Equal(t, "custom", name)
		assert.Equal(t, "desc", cfg.Description)
		assert.Equal(t, "readonly", cfg.Identity)
		assert.Equal(t, "30s", cfg.Timeout)
		assert.True(t, cfg.AutoStart)
		assert.Equal(t, map[string]string{"K": "V"}, cfg.Env)
		assert.Equal(t, map[string]string{"H": "v"}, cfg.Headers)
	})

	t.Run("resolves preset by name, ignoring transport parsing", func(t *testing.T) {
		name, cfg, err := ParseServerSpec(atmosConfig, ServerSpec{Target: "self"})
		require.NoError(t, err)
		assert.Equal(t, "atmos", name)
		assert.Equal(t, "atmos", cfg.Command)
		assert.Equal(t, []string{"mcp", "start"}, cfg.Args)
	})

	t.Run("preset name override", func(t *testing.T) {
		name, _, err := ParseServerSpec(atmosConfig, ServerSpec{Target: "atmos-pro", Name: "my-pro"})
		require.NoError(t, err)
		assert.Equal(t, "my-pro", name)
	})
}

func TestWrite_NewKey(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("# a comment\nbase_path: \"./\"\n"), 0o600))

	err := Write(file, "aws-docs", schema.MCPServerConfig{
		Command: "uvx",
		Args:    []string{"awslabs.aws-docs@latest"},
	})
	require.NoError(t, err)

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "# a comment", "comments must be preserved")
	assert.Contains(t, content, "aws-docs")
	assert.Contains(t, content, "uvx")
}

func TestWrite_OverwritesSubtree(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("mcp:\n  servers:\n    test:\n      command: old\n      args: [\"a\"]\n"), 0o600))

	// Switching a server from stdio to http must not leave stale command/args behind.
	err := Write(file, "test", schema.MCPServerConfig{
		Type: schema.MCPTransportHTTP,
		URL:  "https://mcp.example.com/mcp",
	})
	require.NoError(t, err)

	exists, err := Exists(file, "test")
	require.NoError(t, err)
	assert.True(t, exists)

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "https://mcp.example.com/mcp")
	assert.NotContains(t, content, "old", "stale command field must not survive an overwrite")
}

func TestRemove_ExistingKey(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("mcp:\n  servers:\n    keep:\n      command: a\n    drop:\n      command: b\n"), 0o600))

	require.NoError(t, Remove(file, "drop"))

	exists, err := Exists(file, "drop")
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = Exists(file, "keep")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRemove_MissingKeyIsNoop(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("mcp:\n  servers:\n    keep:\n      command: a\n"), 0o600))

	require.NoError(t, Remove(file, "missing"))

	exists, err := Exists(file, "keep")
	require.NoError(t, err)
	assert.True(t, exists, "unrelated keys must be untouched")
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("mcp:\n  servers:\n    present:\n      command: a\n"), 0o600))

	exists, err := Exists(file, "present")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = Exists(file, "absent")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestHasServerWithURL(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{
		"renamed-pro": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp"},
		"aws-docs":    {Command: "uvx"},
	}
	assert.True(t, HasServerWithURL(servers, "https://atmos-pro.com/mcp"))
	assert.False(t, HasServerWithURL(servers, "https://other.example.com/mcp"))
}

func TestResolveFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("base_path: \"./\"\n"), 0o600))

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("config", nil, "")
	require.NoError(t, cmd.Flags().Set("config", file))

	resolved, err := ResolveFile(cmd, &schema.AtmosConfiguration{})
	require.NoError(t, err)
	assert.Equal(t, file, resolved)
}
