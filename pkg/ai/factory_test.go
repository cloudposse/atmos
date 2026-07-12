package ai

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	// Import providers to register them.
	_ "github.com/cloudposse/atmos/pkg/ai/agent/anthropic"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/azureopenai"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/bedrock"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/claudecode"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/codexcli"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/copilotcli"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/gemini"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/geminicli"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/github"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/grok"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/ollama"
	_ "github.com/cloudposse/atmos/pkg/ai/agent/openai"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name: "No AI settings",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{},
			},
			expectError: true,
			errorMsg:    "AI features are disabled in configuration",
		},
		{
			name: "Anthropic provider (explicit)",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled:         true,
					DefaultProvider: "anthropic",
					Providers: map[string]*schema.AIProviderConfig{
						"anthropic": {},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Anthropic provider (default)",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled: true,
				},
			},
			expectError: false,
		},
		{
			name: "Unsupported provider",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled:         true,
					DefaultProvider: "unsupported",
					Providers: map[string]*schema.AIProviderConfig{
						"unsupported": {},
					},
				},
			},
			expectError: true,
			errorMsg:    "unsupported AI provider: unsupported",
		},
		{
			name: "OpenAI provider",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled:         true,
					DefaultProvider: "openai",
					Providers: map[string]*schema.AIProviderConfig{
						"openai": {},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Gemini provider",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled:         true,
					DefaultProvider: "gemini",
					Providers: map[string]*schema.AIProviderConfig{
						"gemini": {},
					},
				},
			},
			expectError: false,
		},
		{
			name: "GitHub Models provider",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled:         true,
					DefaultProvider: "github",
					Providers: map[string]*schema.AIProviderConfig{
						"github": {},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Grok provider",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled:         true,
					DefaultProvider: "grok",
					Providers: map[string]*schema.AIProviderConfig{
						"grok": {},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Ollama provider",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled:         true,
					DefaultProvider: "ollama",
					Providers: map[string]*schema.AIProviderConfig{
						"ollama": {},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Disabled AI",
			atmosConfig: &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Enabled: false,
				},
			},
			expectError: true,
			errorMsg:    "AI features are disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.atmosConfig)

			// Handle expected error cases.
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, client)
				return
			}

			// Handle success cases.
			// Note: These tests require API key to be set.
			// We're testing the factory routing logic, not the actual client creation.
			if err == nil {
				assert.NotNil(t, client)
				return
			}

			// Check for expected errors when API key is not set.
			errMsg := err.Error()
			if strings.Contains(errMsg, "AI features are disabled") ||
				strings.Contains(errMsg, "API key not found") ||
				strings.Contains(errMsg, "base URL is required") ||
				strings.Contains(errMsg, "failed to load AWS configuration") {
				t.Skipf("Skipping test: %s (expected for factory test without API key)", errMsg)
				return
			}

			// Unexpected error.
			assert.NoError(t, err)
			assert.NotNil(t, client)
		})
	}
}

func TestDetectCLIProvider(t *testing.T) {
	tests := []struct {
		name     string
		lookup   func(string) (string, error)
		expected string
	}{
		{
			name: "claude found returns claude-code (first priority)",
			lookup: func(bin string) (string, error) {
				if bin == "claude" {
					return "/usr/local/bin/claude", nil
				}
				return "", errNotFound
			},
			expected: "claude-code",
		},
		{
			name: "codex found returns codex-cli",
			lookup: func(bin string) (string, error) {
				if bin == "codex" {
					return "/usr/local/bin/codex", nil
				}
				return "", errNotFound
			},
			expected: "codex-cli",
		},
		{
			name: "copilot found returns copilot-cli",
			lookup: func(bin string) (string, error) {
				if bin == "copilot" {
					return "/usr/local/bin/copilot", nil
				}
				return "", errNotFound
			},
			expected: "copilot-cli",
		},
		{
			name: "gemini found returns gemini-cli",
			lookup: func(bin string) (string, error) {
				if bin == "gemini" {
					return "/usr/local/bin/gemini", nil
				}
				return "", errNotFound
			},
			expected: "gemini-cli",
		},
		{
			name: "claude and codex both found returns claude-code (priority order)",
			lookup: func(bin string) (string, error) {
				if bin == "claude" || bin == "codex" {
					return "/usr/local/bin/" + bin, nil
				}
				return "", errNotFound
			},
			expected: "claude-code",
		},
		{
			name: "none found returns empty string",
			lookup: func(string) (string, error) {
				return "", errNotFound
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, detectCLIProvider(tt.lookup))
		})
	}
}

var errNotFound = errors.New("binary not found")

func TestGetProvider(t *testing.T) {
	t.Run("explicit DefaultProvider always wins over PATH", func(t *testing.T) {
		// Force PATH to an empty directory so no real CLI binary can be found, then confirm
		// an explicit DefaultProvider is still returned regardless.
		t.Setenv("PATH", t.TempDir())
		atmosConfig := &schema.AtmosConfiguration{
			AI: schema.AISettings{DefaultProvider: "openai"},
		}
		assert.Equal(t, "openai", GetProvider(atmosConfig))
	})

	t.Run("no DefaultProvider and no CLI tool on PATH falls back to anthropic", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		atmosConfig := &schema.AtmosConfiguration{AI: schema.AISettings{}}
		assert.Equal(t, "anthropic", GetProvider(atmosConfig))
	})
}

func TestIsCLIProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		expected bool
	}{
		{"claude-code is CLI", "claude-code", true},
		{"codex-cli is CLI", "codex-cli", true},
		{"copilot-cli is CLI", "copilot-cli", true},
		{"gemini-cli is CLI", "gemini-cli", true},
		{"anthropic is not CLI", "anthropic", false},
		{"openai is not CLI", "openai", false},
		{"empty is not CLI", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsCLIProvider(tt.provider))
		})
	}
}
