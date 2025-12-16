package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCreateRegistry_RefValidation tests that ref is only allowed with GitHub URLs.
func TestCreateRegistry_RefValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      schema.ToolchainRegistry
		wantErr     bool
		errContains string
	}{
		{
			name: "ref without source should error",
			config: schema.ToolchainRegistry{
				Type: "aqua",
				Ref:  "v1.0.0",
			},
			wantErr:     true,
			errContains: "'ref' requires 'source' to be set",
		},
		{
			name: "ref with github.com URL should succeed",
			config: schema.ToolchainRegistry{
				Type:   "aqua",
				Source: "https://github.com/myorg/registry",
				Ref:    "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "ref with raw.githubusercontent.com URL should error",
			config: schema.ToolchainRegistry{
				Type:   "aqua",
				Source: "https://raw.githubusercontent.com/myorg/registry/main/registry.yaml",
				Ref:    "v1.0.0",
			},
			wantErr:     true,
			errContains: "'ref' is only supported for github.com URLs",
		},
		{
			name: "ref with non-GitHub URL should error",
			config: schema.ToolchainRegistry{
				Type:   "aqua",
				Source: "https://example.com/registry.yaml",
				Ref:    "v1.0.0",
			},
			wantErr:     true,
			errContains: "'ref' is only supported for github.com URLs",
		},
		{
			name: "no ref with any URL should succeed",
			config: schema.ToolchainRegistry{
				Type:   "aqua",
				Source: "https://example.com/registry.yaml",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := createRegistry(&tt.config)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrRegistryConfiguration)
			if tt.errContains != "" {
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

// TestIsGitHubURL tests the isGitHubURL helper function.
func TestIsGitHubURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://github.com/owner/repo", true},
		{"https://github.com/owner/repo/path/file.yaml", true},
		{"https://raw.githubusercontent.com/owner/repo/main/file.yaml", false},
		{"https://example.com/registry.yaml", false},
		{"https://gitlab.com/owner/repo", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := isGitHubURL(tt.url); got != tt.want {
				t.Errorf("isGitHubURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

