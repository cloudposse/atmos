package registry

import (
	"errors"
	"testing"

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
			if tt.wantErr {
				if err == nil {
					t.Errorf("createRegistry() expected error, got nil")
					return
				}
				if !errors.Is(err, ErrRegistryConfiguration) {
					t.Errorf("createRegistry() error = %v, want ErrRegistryConfiguration", err)
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("createRegistry() error = %q, want to contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("createRegistry() unexpected error = %v", err)
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

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
