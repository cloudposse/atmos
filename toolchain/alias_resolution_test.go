package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultToolResolver_AliasResolution(t *testing.T) {
	tests := []struct {
		name       string
		aliases    map[string]string
		toolName   string
		wantOwner  string
		wantRepo   string
		wantErr    bool
		errMessage string
	}{
		{
			name: "resolve alias terraform to hashicorp/terraform",
			aliases: map[string]string{
				"terraform": "hashicorp/terraform",
			},
			toolName:  "terraform",
			wantOwner: "hashicorp",
			wantRepo:  "terraform",
			wantErr:   false,
		},
		{
			name: "resolve alias tf to hashicorp/terraform",
			aliases: map[string]string{
				"tf": "hashicorp/terraform",
			},
			toolName:  "tf",
			wantOwner: "hashicorp",
			wantRepo:  "terraform",
			wantErr:   false,
		},
		{
			name: "resolve alias tofu to opentofu/opentofu",
			aliases: map[string]string{
				"tofu": "opentofu/opentofu",
			},
			toolName:  "tofu",
			wantOwner: "opentofu",
			wantRepo:  "opentofu",
			wantErr:   false,
		},
		{
			name: "multiple aliases pointing to same tool",
			aliases: map[string]string{
				"terraform": "hashicorp/terraform",
				"tf":        "hashicorp/terraform",
			},
			toolName:  "tf",
			wantOwner: "hashicorp",
			wantRepo:  "terraform",
			wantErr:   false,
		},
		{
			name:      "no alias - already owner/repo format",
			aliases:   map[string]string{},
			toolName:  "hashicorp/terraform",
			wantOwner: "hashicorp",
			wantRepo:  "terraform",
			wantErr:   false,
		},
		{
			name: "alias not found - fallback to registry search (will fail in test)",
			aliases: map[string]string{
				"terraform": "hashicorp/terraform",
			},
			toolName:   "nonexistent",
			wantErr:    true,
			errMessage: "not found in Aqua registry",
		},
		{
			name:      "nil aliases map - should handle gracefully",
			aliases:   nil,
			toolName:  "hashicorp/terraform",
			wantOwner: "hashicorp",
			wantRepo:  "terraform",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a resolver with the test aliases.
			resolver := &DefaultToolResolver{
				atmosConfig: &schema.AtmosConfiguration{
					Toolchain: schema.Toolchain{
						Aliases: tt.aliases,
					},
				},
			}

			owner, repo, err := resolver.Resolve(tt.toolName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestDefaultToolResolver_AliasResolution_NoConfig(t *testing.T) {
	// Test that resolver works when atmosConfig is nil.
	resolver := &DefaultToolResolver{
		atmosConfig: nil,
	}

	// Should handle gracefully and parse owner/repo format directly.
	owner, repo, err := resolver.Resolve("hashicorp/terraform")
	require.NoError(t, err)
	assert.Equal(t, "hashicorp", owner)
	assert.Equal(t, "terraform", repo)
}

func TestDefaultToolResolver_AliasChaining(t *testing.T) {
	// Test that aliases are resolved in a single step (no chaining).
	// If "myshort" -> "mymedium" and "mymedium" -> "owner/repo",
	// "myshort" should resolve to "mymedium" (the first alias value), not "owner/repo".
	// This is the expected behavior - aliases are not transitive.
	resolver := &DefaultToolResolver{
		atmosConfig: &schema.AtmosConfiguration{
			Toolchain: schema.Toolchain{
				Aliases: map[string]string{
					"myshort":  "mymedium",
					"mymedium": "owner/repo",
				},
			},
		},
	}

	// "myshort" should resolve to the literal value "mymedium", not chase the chain.
	// Since "mymedium" is just a name (not owner/repo), it will try Aqua registry and fail.
	_, _, err := resolver.Resolve("myshort")
	require.Error(t, err, "myshort resolves to 'mymedium' which is not owner/repo, should fail registry lookup")
	assert.Contains(t, err.Error(), "not found in Aqua registry")

	// But "mymedium" should resolve properly to owner/repo.
	owner, repo, err := resolver.Resolve("mymedium")
	require.NoError(t, err)
	assert.Equal(t, "owner", owner)
	assert.Equal(t, "repo", repo)
}

func TestDefaultToolResolver_AliasWithSlash(t *testing.T) {
	// Test that if an alias value contains a slash, it's treated as owner/repo.
	resolver := &DefaultToolResolver{
		atmosConfig: &schema.AtmosConfiguration{
			Toolchain: schema.Toolchain{
				Aliases: map[string]string{
					"myalias": "custom/tool",
				},
			},
		},
	}

	owner, repo, err := resolver.Resolve("myalias")
	require.NoError(t, err)
	assert.Equal(t, "custom", owner)
	assert.Equal(t, "tool", repo)
}
