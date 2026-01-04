package atmos

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/toolchain/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseToolID tests the pure function for parsing tool identifiers.
func TestParseToolID(t *testing.T) {
	tests := []struct {
		name      string
		toolID    string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "valid tool ID",
			toolID:    "stedolan/jq",
			wantOwner: "stedolan",
			wantRepo:  "jq",
			wantErr:   false,
		},
		{
			name:      "valid tool ID with spaces",
			toolID:    " stedolan / jq ",
			wantOwner: "stedolan",
			wantRepo:  "jq",
			wantErr:   false,
		},
		{
			name:      "missing repo",
			toolID:    "stedolan/",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
		{
			name:      "missing owner",
			toolID:    "/jq",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
		{
			name:      "no slash",
			toolID:    "stedolan",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
		{
			name:      "multiple slashes",
			toolID:    "stedolan/jq/extra",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
		{
			name:      "empty string",
			toolID:    "",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parseToolID(tt.toolID)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

// TestParseToolConfig tests the pure function for parsing tool configuration.
func TestParseToolConfig(t *testing.T) {
	tests := []struct {
		name    string
		owner   string
		repo    string
		config  map[string]any
		want    *registry.Tool
		wantErr bool
	}{
		{
			name:  "minimal valid config",
			owner: "stedolan",
			repo:  "jq",
			config: map[string]any{
				"type": "github_release",
				"url":  "jq-{{.OS}}-{{.Arch}}",
			},
			want: &registry.Tool{
				RepoOwner: "stedolan",
				RepoName:  "jq",
				Name:      "jq",
				Type:      "github_release",
				URL:       "jq-{{.OS}}-{{.Arch}}",
				Asset:     "jq-{{.OS}}-{{.Arch}}",
				Registry:  "atmos-inline",
			},
			wantErr: false,
		},
		{
			name:  "full config with optional fields",
			owner: "mikefarah",
			repo:  "yq",
			config: map[string]any{
				"type":        "github_release",
				"url":         "yq_{{.OS}}_{{.Arch}}",
				"format":      "tar.gz",
				"binary_name": "yq",
			},
			want: &registry.Tool{
				RepoOwner:  "mikefarah",
				RepoName:   "yq",
				Name:       "yq",
				Type:       "github_release",
				URL:        "yq_{{.OS}}_{{.Arch}}",
				Asset:      "yq_{{.OS}}_{{.Arch}}",
				Format:     "tar.gz",
				BinaryName: "yq",
				Registry:   "atmos-inline",
			},
			wantErr: false,
		},
		{
			name:  "http type tool",
			owner: "example",
			repo:  "tool",
			config: map[string]any{
				"type": "http",
				"url":  "https://example.com/tool-{{.Version}}.tar.gz",
			},
			want: &registry.Tool{
				RepoOwner: "example",
				RepoName:  "tool",
				Name:      "tool",
				Type:      "http",
				URL:       "https://example.com/tool-{{.Version}}.tar.gz",
				Asset:     "https://example.com/tool-{{.Version}}.tar.gz", // Asset is set for template rendering.
				Registry:  "atmos-inline",
			},
			wantErr: false,
		},
		{
			name:  "missing type field",
			owner: "stedolan",
			repo:  "jq",
			config: map[string]any{
				"url": "jq-{{.OS}}-{{.Arch}}",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:  "missing url field",
			owner: "stedolan",
			repo:  "jq",
			config: map[string]any{
				"type": "github_release",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:  "invalid type field type",
			owner: "stedolan",
			repo:  "jq",
			config: map[string]any{
				"type": 123,
				"url":  "jq-{{.OS}}-{{.Arch}}",
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseToolConfig(tt.owner, tt.repo, tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestParseToolDefinitions tests the pure function for parsing all tool definitions.
func TestParseToolDefinitions(t *testing.T) {
	tests := []struct {
		name        string
		toolsConfig map[string]any
		wantCount   int
		wantTools   []string // Tool IDs that should exist.
		wantErr     bool
	}{
		{
			name: "valid multiple tools",
			toolsConfig: map[string]any{
				"stedolan/jq": map[string]any{
					"type": "github_release",
					"url":  "jq-{{.OS}}-{{.Arch}}",
				},
				"mikefarah/yq": map[string]any{
					"type": "github_release",
					"url":  "yq_{{.OS}}_{{.Arch}}",
				},
			},
			wantCount: 2,
			wantTools: []string{"stedolan/jq", "mikefarah/yq"},
			wantErr:   false,
		},
		{
			name: "single tool",
			toolsConfig: map[string]any{
				"stedolan/jq": map[string]any{
					"type": "github_release",
					"url":  "jq-{{.OS}}-{{.Arch}}",
				},
			},
			wantCount: 1,
			wantTools: []string{"stedolan/jq"},
			wantErr:   false,
		},
		{
			name:        "empty tools config",
			toolsConfig: map[string]any{},
			wantCount:   0,
			wantTools:   []string{},
			wantErr:     false,
		},
		{
			name: "invalid tool ID",
			toolsConfig: map[string]any{
				"invalid": map[string]any{
					"type": "github_release",
					"url":  "tool-{{.OS}}",
				},
			},
			wantCount: 0,
			wantTools: nil,
			wantErr:   true,
		},
		{
			name: "invalid tool config type",
			toolsConfig: map[string]any{
				"stedolan/jq": "not a map",
			},
			wantCount: 0,
			wantTools: nil,
			wantErr:   true,
		},
		{
			name: "invalid tool config content",
			toolsConfig: map[string]any{
				"stedolan/jq": map[string]any{
					"type": "github_release",
					// Missing url field.
				},
			},
			wantCount: 0,
			wantTools: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseToolDefinitions(tt.toolsConfig)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, len(got))

			for _, toolID := range tt.wantTools {
				assert.Contains(t, got, toolID, "Expected tool %s to exist", toolID)
			}
		})
	}
}

// TestNewAtmosRegistry tests registry construction.
func TestNewAtmosRegistry(t *testing.T) {
	t.Run("valid registry creation", func(t *testing.T) {
		toolsConfig := map[string]any{
			"stedolan/jq": map[string]any{
				"type": "github_release",
				"url":  "jq-{{.OS}}-{{.Arch}}",
			},
			"mikefarah/yq": map[string]any{
				"type": "github_release",
				"url":  "yq_{{.OS}}_{{.Arch}}",
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)
		assert.NotNil(t, reg)
		assert.Equal(t, 2, len(reg.tools))
	})

	t.Run("invalid config fails", func(t *testing.T) {
		toolsConfig := map[string]any{
			"invalid": "not a map",
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		assert.Error(t, err)
		assert.Nil(t, reg)
	})
}

// TestAtmosRegistry_GetTool tests tool retrieval.
func TestAtmosRegistry_GetTool(t *testing.T) {
	toolsConfig := map[string]any{
		"stedolan/jq": map[string]any{
			"type": "github_release",
			"url":  "jq-{{.OS}}-{{.Arch}}",
		},
	}

	reg, err := NewAtmosRegistry(toolsConfig)
	require.NoError(t, err)

	t.Run("get existing tool", func(t *testing.T) {
		tool, err := reg.GetTool("stedolan", "jq")
		require.NoError(t, err)
		assert.Equal(t, "stedolan", tool.RepoOwner)
		assert.Equal(t, "jq", tool.RepoName)
	})

	t.Run("get non-existent tool", func(t *testing.T) {
		tool, err := reg.GetTool("nonexistent", "tool")
		assert.Error(t, err)
		assert.Nil(t, tool)
		assert.ErrorIs(t, err, registry.ErrToolNotFound)
	})
}

// TestAtmosRegistry_GetToolWithVersion tests tool retrieval with version.
func TestAtmosRegistry_GetToolWithVersion(t *testing.T) {
	toolsConfig := map[string]any{
		"stedolan/jq": map[string]any{
			"type": "github_release",
			"url":  "jq-{{.OS}}-{{.Arch}}",
		},
	}

	reg, err := NewAtmosRegistry(toolsConfig)
	require.NoError(t, err)

	tool, err := reg.GetToolWithVersion("stedolan", "jq", "1.7.1")
	require.NoError(t, err)
	assert.Equal(t, "1.7.1", tool.Version)
}

// TestAtmosRegistry_Search tests search functionality.
func TestAtmosRegistry_Search(t *testing.T) {
	toolsConfig := map[string]any{
		"stedolan/jq": map[string]any{
			"type": "github_release",
			"url":  "jq-{{.OS}}-{{.Arch}}",
		},
		"mikefarah/yq": map[string]any{
			"type": "github_release",
			"url":  "yq_{{.OS}}_{{.Arch}}",
		},
	}

	reg, err := NewAtmosRegistry(toolsConfig)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("search by repo name", func(t *testing.T) {
		results, err := reg.Search(ctx, "jq")
		require.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, "jq", results[0].RepoName)
	})

	t.Run("search by owner name", func(t *testing.T) {
		results, err := reg.Search(ctx, "mikefarah")
		require.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, "yq", results[0].RepoName)
	})

	t.Run("search with no results", func(t *testing.T) {
		results, err := reg.Search(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Equal(t, 0, len(results))
	})
}

// TestAtmosRegistry_ListAll tests listing all tools.
func TestAtmosRegistry_ListAll(t *testing.T) {
	toolsConfig := map[string]any{
		"stedolan/jq": map[string]any{
			"type": "github_release",
			"url":  "jq-{{.OS}}-{{.Arch}}",
		},
		"mikefarah/yq": map[string]any{
			"type": "github_release",
			"url":  "yq_{{.OS}}_{{.Arch}}",
		},
	}

	reg, err := NewAtmosRegistry(toolsConfig)
	require.NoError(t, err)

	ctx := context.Background()
	tools, err := reg.ListAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, len(tools))
}

// TestAtmosRegistry_GetMetadata tests metadata retrieval.
func TestAtmosRegistry_GetMetadata(t *testing.T) {
	toolsConfig := map[string]any{
		"stedolan/jq": map[string]any{
			"type": "github_release",
			"url":  "jq-{{.OS}}-{{.Arch}}",
		},
	}

	reg, err := NewAtmosRegistry(toolsConfig)
	require.NoError(t, err)

	ctx := context.Background()
	metadata, err := reg.GetMetadata(ctx)
	require.NoError(t, err)
	assert.Equal(t, "atmos-inline", metadata.Name)
	assert.Equal(t, "atmos", metadata.Type)
	assert.Equal(t, 1, metadata.ToolCount)
}

// TestAtmosRegistry_GetLatestVersion tests that version queries are not supported.
func TestAtmosRegistry_GetLatestVersion(t *testing.T) {
	toolsConfig := map[string]any{
		"stedolan/jq": map[string]any{
			"type": "github_release",
			"url":  "jq-{{.OS}}-{{.Arch}}",
		},
	}

	reg, err := NewAtmosRegistry(toolsConfig)
	require.NoError(t, err)

	version, err := reg.GetLatestVersion("stedolan", "jq")
	assert.Error(t, err)
	assert.Empty(t, version)
	assert.ErrorIs(t, err, registry.ErrNoVersionsFound)
}
