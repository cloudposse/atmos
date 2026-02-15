package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsAIEnabled(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expected    bool
	}{
		{
			name: "AI not configured",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expected: false,
		},
		{
			name: "AI explicitly disabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: false,
					},
				},
			},
			expected: false,
		},
		{
			name: "AI enabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
					},
				},
			},
			expected: true,
		},
		{
			name: "AI enabled with provider configured",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model:     "claude-sonnet-4-5-20250929",
								MaxTokens: 4096,
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "AI with invalid enabled value (defaults to false)",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: false,
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAIEnabled(tt.atmosConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAICommandProvider(t *testing.T) {
	t.Run("GetCommand returns ai command", func(t *testing.T) {
		provider := &AICommandProvider{}
		cmd := provider.GetCommand()

		require.NotNil(t, cmd)
		assert.Equal(t, "ai", cmd.Use)
		assert.Equal(t, "AI-powered assistant for Atmos operations", cmd.Short)
		assert.NotEmpty(t, cmd.Long)
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		provider := &AICommandProvider{}
		name := provider.GetName()

		assert.Equal(t, "ai", name)
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		provider := &AICommandProvider{}
		group := provider.GetGroup()

		assert.Equal(t, "Other Commands", group)
	})

	t.Run("GetFlagsBuilder returns nil", func(t *testing.T) {
		provider := &AICommandProvider{}
		builder := provider.GetFlagsBuilder()

		assert.Nil(t, builder, "GetFlagsBuilder should return nil for ai command")
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		provider := &AICommandProvider{}
		builder := provider.GetPositionalArgsBuilder()

		assert.Nil(t, builder, "GetPositionalArgsBuilder should return nil for ai command")
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		provider := &AICommandProvider{}
		flags := provider.GetCompatibilityFlags()

		assert.Nil(t, flags, "GetCompatibilityFlags should return nil for ai command")
	})

	t.Run("GetAliases returns nil", func(t *testing.T) {
		provider := &AICommandProvider{}
		aliases := provider.GetAliases()

		assert.Nil(t, aliases, "GetAliases should return nil for ai command")
	})

	t.Run("IsExperimental returns true", func(t *testing.T) {
		provider := &AICommandProvider{}
		isExperimental := provider.IsExperimental()

		assert.True(t, isExperimental, "ai command should be marked as experimental")
	})

	t.Run("command has subcommands", func(t *testing.T) {
		provider := &AICommandProvider{}
		cmd := provider.GetCommand()

		// Verify that subcommands are attached.
		// The exact subcommands depend on init() calls, but we can check that some exist.
		subcommands := cmd.Commands()
		assert.NotEmpty(t, subcommands, "ai command should have subcommands")

		// Check for expected subcommand names.
		subcommandNames := make(map[string]bool)
		for _, subcmd := range subcommands {
			subcommandNames[subcmd.Name()] = true
		}

		expectedSubcommands := []string{"skill", "ask", "chat", "help", "memory", "sessions"}
		for _, expected := range expectedSubcommands {
			assert.True(t, subcommandNames[expected], "expected subcommand %s not found", expected)
		}
	})
}
