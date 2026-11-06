package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestInitCliConfig_AIProviderCacheSettings checks the documented nested `cache` block under
// ai.providers.<name> is decoded from atmos.yaml, and that leaving it out keeps Cache nil so the
// Anthropic and Bedrock providers fall back to their defaults (caching on).
func TestInitCliConfig_AIProviderCacheSettings(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		yaml     string
		want     *schema.AICacheSettings
	}{
		{
			name:     "no cache block leaves Cache nil",
			provider: "anthropic",
			yaml: `
ai:
  enabled: true
  providers:
    anthropic:
      model: claude-sonnet-4-6
`,
			want: nil,
		},
		{
			name:     "nested cache block with selective settings",
			provider: "anthropic",
			yaml: `
ai:
  enabled: true
  providers:
    anthropic:
      model: claude-sonnet-4-6
      cache:
        enabled: true
        cache_system_prompt: true
        cache_project_instructions: false
`,
			want: &schema.AICacheSettings{Enabled: true, CacheSystemPrompt: true, CacheProjectInstructions: false},
		},
		{
			name:     "nested cache block disabled",
			provider: "bedrock",
			yaml: `
ai:
  enabled: true
  providers:
    bedrock:
      model: global.anthropic.claude-sonnet-5
      cache:
        enabled: false
`,
			want: &schema.AICacheSettings{Enabled: false},
		},
		{
			name:     "nested cache block for bedrock caching both blocks",
			provider: "bedrock",
			yaml: `
ai:
  enabled: true
  providers:
    bedrock:
      model: global.anthropic.claude-sonnet-5
      cache:
        enabled: true
        cache_system_prompt: true
        cache_project_instructions: true
`,
			want: &schema.AICacheSettings{Enabled: true, CacheSystemPrompt: true, CacheProjectInstructions: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("base_path: .\n"+tt.yaml), 0o644))
			t.Chdir(dir)

			atmosConfig, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)

			require.NoError(t, err)
			providerConfig := atmosConfig.AI.Providers[tt.provider]
			require.NotNil(t, providerConfig)
			assert.Equal(t, tt.want, providerConfig.Cache)
		})
	}
}
