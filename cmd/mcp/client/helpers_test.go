package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: fails build if referenced schema fields are renamed.
var _ = schema.MCPServerConfig{
	Command:  "",
	Args:     nil,
	Env:      nil,
	Identity: "",
}

// TestFirstSentence tests the firstSentence helper that extracts the first sentence
// from a description string, collapsing whitespace and handling markdown boundaries.
func TestFirstSentence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal sentence with period and space",
			input:    "First sentence. Second sentence.",
			expected: "First sentence.",
		},
		{
			name:     "no period in text",
			input:    "Just a phrase",
			expected: "Just a phrase",
		},
		{
			name:     "markdown header boundary",
			input:    "Some text ## Header",
			expected: "Some text.",
		},
		{
			name:     "multi-line input with whitespace collapsed",
			input:    "Line one\nLine two. Line three.",
			expected: "Line one Line two.",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single period at end without trailing space",
			input:    "Only sentence.",
			expected: "Only sentence.",
		},
		{
			name:     "period at end followed by space",
			input:    "Only sentence. ",
			expected: "Only sentence.",
		},
		{
			name:     "tabs and multiple spaces collapsed",
			input:    "Word1\t\tWord2   Word3. Rest.",
			expected: "Word1 Word2 Word3.",
		},
		{
			name:     "markdown header without preceding text",
			input:    "## Header only",
			expected: "## Header only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firstSentence(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildMCPJSONEntry tests the buildMCPJSONEntry function that creates .mcp.json
// entries from MCPServerConfig, with optional identity wrapping.
func TestBuildMCPJSONEntry(t *testing.T) {
	t.Run("server without identity uses command directly", func(t *testing.T) {
		cfg := &schema.MCPServerConfig{
			Command: "npx",
			Args:    []string{"-y", "some-mcp-server"},
			Env:     map[string]string{"API_KEY": "abc123"},
		}

		entry := buildMCPJSONEntry("my-server", cfg)

		assert.Equal(t, "npx", entry.Command)
		assert.Equal(t, []string{"-y", "some-mcp-server"}, entry.Args)
		assert.Equal(t, map[string]string{"API_KEY": "abc123"}, entry.Env)
	})

	t.Run("server with identity wraps with atmos auth exec", func(t *testing.T) {
		cfg := &schema.MCPServerConfig{
			Command:  "npx",
			Args:     []string{"-y", "some-mcp-server"},
			Env:      map[string]string{"REGION": "us-east-1"},
			Identity: "aws-dev",
		}

		entry := buildMCPJSONEntry("my-server", cfg)

		assert.Equal(t, "atmos", entry.Command)
		// Should be: atmos auth exec -i aws-dev -- npx -y some-mcp-server.
		expectedArgs := []string{"auth", "exec", "-i", "aws-dev", "--", "npx", "-y", "some-mcp-server"}
		assert.Equal(t, expectedArgs, entry.Args)
		assert.Equal(t, map[string]string{"REGION": "us-east-1"}, entry.Env)
	})

	t.Run("server with no env has nil env", func(t *testing.T) {
		cfg := &schema.MCPServerConfig{
			Command: "echo",
			Args:    []string{"hello"},
		}

		entry := buildMCPJSONEntry("simple", cfg)

		assert.Equal(t, "echo", entry.Command)
		assert.Equal(t, []string{"hello"}, entry.Args)
		assert.Nil(t, entry.Env)
	})

	t.Run("server with identity and no args", func(t *testing.T) {
		cfg := &schema.MCPServerConfig{
			Command:  "my-server",
			Identity: "prod-identity",
		}

		entry := buildMCPJSONEntry("prod", cfg)

		assert.Equal(t, "atmos", entry.Command)
		expectedArgs := []string{"auth", "exec", "-i", "prod-identity", "--", "my-server"}
		assert.Equal(t, expectedArgs, entry.Args)
	})

	t.Run("server with empty env map", func(t *testing.T) {
		cfg := &schema.MCPServerConfig{
			Command: "echo",
			Env:     map[string]string{},
		}

		entry := buildMCPJSONEntry("test", cfg)

		assert.Equal(t, "echo", entry.Command)
		// Empty map is preserved (not nil).
		require.NotNil(t, entry.Env)
		assert.Empty(t, entry.Env)
	})
}
