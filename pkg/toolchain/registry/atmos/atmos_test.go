package atmos

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
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

// TestAtmosRegistry_OverridesSupport tests platform-specific override parsing.
func TestAtmosRegistry_OverridesSupport(t *testing.T) {
	t.Run("parses overrides correctly", func(t *testing.T) {
		toolsConfig := map[string]any{
			"replicatedhq/replicated": map[string]any{
				"type":  "github_release",
				"asset": "replicated_{{trimV .Version}}_{{.OS}}_{{.Arch}}.tar.gz",
				"overrides": []any{
					map[string]any{
						"goos":  "darwin",
						"asset": "replicated_{{trimV .Version}}_{{.OS}}_all.tar.gz",
					},
				},
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("replicatedhq", "replicated")
		require.NoError(t, err)
		require.NotNil(t, tool)

		// Verify overrides were parsed.
		require.Len(t, tool.Overrides, 1)
		assert.Equal(t, "darwin", tool.Overrides[0].GOOS)
		assert.Equal(t, "", tool.Overrides[0].GOARCH)
		assert.Equal(t, "replicated_{{trimV .Version}}_{{.OS}}_all.tar.gz", tool.Overrides[0].Asset)
	})

	t.Run("parses multiple overrides", func(t *testing.T) {
		toolsConfig := map[string]any{
			"owner/tool": map[string]any{
				"type":  "github_release",
				"asset": "tool-{{.OS}}-{{.Arch}}.tar.gz",
				"overrides": []any{
					map[string]any{
						"goos":   "darwin",
						"goarch": "arm64",
						"asset":  "tool-macos-arm64.tar.gz",
						"format": "tar.gz",
					},
					map[string]any{
						"goos":   "windows",
						"asset":  "tool-windows.zip",
						"format": "zip",
					},
				},
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("owner", "tool")
		require.NoError(t, err)
		require.Len(t, tool.Overrides, 2)

		// First override.
		assert.Equal(t, "darwin", tool.Overrides[0].GOOS)
		assert.Equal(t, "arm64", tool.Overrides[0].GOARCH)
		assert.Equal(t, "tool-macos-arm64.tar.gz", tool.Overrides[0].Asset)
		assert.Equal(t, "tar.gz", tool.Overrides[0].Format)

		// Second override.
		assert.Equal(t, "windows", tool.Overrides[1].GOOS)
		assert.Equal(t, "tool-windows.zip", tool.Overrides[1].Asset)
		assert.Equal(t, "zip", tool.Overrides[1].Format)
	})

	t.Run("tool without overrides has empty slice", func(t *testing.T) {
		toolsConfig := map[string]any{
			"owner/tool": map[string]any{
				"type":  "github_release",
				"asset": "tool-{{.OS}}-{{.Arch}}.tar.gz",
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("owner", "tool")
		require.NoError(t, err)
		assert.Empty(t, tool.Overrides)
	})

	t.Run("malformed override entries are silently skipped", func(t *testing.T) {
		// Test that malformed override entries (non-map) are skipped without error.
		toolsConfig := map[string]any{
			"owner/tool": map[string]any{
				"type":  "github_release",
				"asset": "tool-{{.OS}}-{{.Arch}}.tar.gz",
				"overrides": []any{
					"not-a-map",                      // Invalid: string instead of map.
					123,                              // Invalid: int instead of map.
					map[string]any{"goos": "darwin"}, // Valid override.
				},
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("owner", "tool")
		require.NoError(t, err)
		// Only the valid override should be parsed.
		require.Len(t, tool.Overrides, 1)
		assert.Equal(t, "darwin", tool.Overrides[0].GOOS)
	})
}

// TestAtmosRegistry_OverridesFilesReplacements tests that files and replacements are parsed within overrides.
// REGRESSION TEST: This test ensures inline atmos registries support files/replacements in platform-specific overrides.
// The Aqua registry correctly handles these (see convertAquaOverrides in aqua.go), but the atmos inline registry
// was missing this functionality in parseOverrides().
func TestAtmosRegistry_OverridesFilesReplacements(t *testing.T) {
	t.Run("parses files in overrides", func(t *testing.T) {
		// Some tools need different file extraction paths per platform.
		// For example, a tool might have different binary locations on macOS vs Linux.
		toolsConfig := map[string]any{
			"aws/aws-cli": map[string]any{
				"type":   "http",
				"url":    "https://awscli.amazonaws.com/awscli-exe-{{.OS}}-{{.Arch}}-{{.Version}}.zip",
				"format": "zip",
				"files": []any{
					map[string]any{
						"name": "aws",
						"src":  "aws/dist/aws",
					},
				},
				"overrides": []any{
					map[string]any{
						"goos": "darwin",
						"files": []any{
							map[string]any{
								"name": "aws",
								"src":  "aws-cli/aws", // Different path on macOS.
							},
						},
					},
				},
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("aws", "aws-cli")
		require.NoError(t, err)

		// Verify base files are parsed.
		require.Len(t, tool.Files, 1, "base files should be parsed")
		assert.Equal(t, "aws/dist/aws", tool.Files[0].Src)

		// Verify override files are parsed.
		require.Len(t, tool.Overrides, 1, "overrides should be parsed")
		require.Len(t, tool.Overrides[0].Files, 1, "files in override should be parsed")
		assert.Equal(t, "aws", tool.Overrides[0].Files[0].Name)
		assert.Equal(t, "aws-cli/aws", tool.Overrides[0].Files[0].Src)
	})

	t.Run("parses replacements in overrides", func(t *testing.T) {
		// Some tools need different arch/os replacements per platform.
		toolsConfig := map[string]any{
			"owner/tool": map[string]any{
				"type":  "github_release",
				"asset": "tool_{{.OS}}_{{.Arch}}.tar.gz",
				"replacements": map[string]any{
					"amd64": "x86_64",
				},
				"overrides": []any{
					map[string]any{
						"goos": "darwin",
						"replacements": map[string]any{
							"amd64": "universal", // Different replacement for macOS.
							"arm64": "universal",
						},
					},
				},
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("owner", "tool")
		require.NoError(t, err)

		// Verify base replacements are parsed.
		require.NotNil(t, tool.Replacements, "base replacements should be parsed")
		assert.Equal(t, "x86_64", tool.Replacements["amd64"])

		// Verify override replacements are parsed.
		require.Len(t, tool.Overrides, 1, "overrides should be parsed")
		require.NotNil(t, tool.Overrides[0].Replacements, "replacements in override should be parsed")
		assert.Equal(t, "universal", tool.Overrides[0].Replacements["amd64"])
		assert.Equal(t, "universal", tool.Overrides[0].Replacements["arm64"])
	})

	t.Run("parses both files and replacements in same override", func(t *testing.T) {
		// Complete scenario: override with all fields including files and replacements.
		toolsConfig := map[string]any{
			"replicatedhq/replicated": map[string]any{
				"type":   "github_release",
				"asset":  "replicated_{{trimV .Version}}_{{.OS}}_{{.Arch}}.tar.gz",
				"format": "tar.gz",
				"files": []any{
					map[string]any{
						"name": "replicated",
						"src":  "replicated",
					},
				},
				"overrides": []any{
					map[string]any{
						"goos":   "darwin",
						"asset":  "replicated_{{trimV .Version}}_darwin_all.tar.gz",
						"format": "tar.gz",
						"files": []any{
							map[string]any{
								"name": "replicated",
								"src":  "darwin/replicated", // Different location on macOS.
							},
						},
						"replacements": map[string]any{
							"arm64": "all",
							"amd64": "all",
						},
					},
				},
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("replicatedhq", "replicated")
		require.NoError(t, err)

		// Verify override has all fields.
		require.Len(t, tool.Overrides, 1)
		override := tool.Overrides[0]

		assert.Equal(t, "darwin", override.GOOS)
		assert.Equal(t, "replicated_{{trimV .Version}}_darwin_all.tar.gz", override.Asset)
		assert.Equal(t, "tar.gz", override.Format)

		// Files in override.
		require.Len(t, override.Files, 1, "files in override should be parsed")
		assert.Equal(t, "replicated", override.Files[0].Name)
		assert.Equal(t, "darwin/replicated", override.Files[0].Src)

		// Replacements in override.
		require.NotNil(t, override.Replacements, "replacements in override should be parsed")
		assert.Equal(t, "all", override.Replacements["arm64"])
		assert.Equal(t, "all", override.Replacements["amd64"])
	})
}

// TestAtmosRegistry_FilesReplacementsVersionPrefix tests parsing of files, replacements, and version_prefix.
// REGRESSION TEST: This test ensures inline atmos registries support all fields needed for tools like replicated.
func TestAtmosRegistry_FilesReplacementsVersionPrefix(t *testing.T) {
	t.Run("parses files configuration", func(t *testing.T) {
		// Test the replicated pattern with files.
		toolsConfig := map[string]any{
			"replicatedhq/replicated": map[string]any{
				"type":   "github_release",
				"asset":  "replicated_{{trimV .Version}}_{{.OS}}_{{.Arch}}.tar.gz",
				"format": "tar.gz",
				"files": []any{
					map[string]any{
						"name": "replicated",
						"src":  "replicated",
					},
				},
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("replicatedhq", "replicated")
		require.NoError(t, err)
		require.Len(t, tool.Files, 1, "files should be parsed")
		assert.Equal(t, "replicated", tool.Files[0].Name)
		assert.Equal(t, "replicated", tool.Files[0].Src)
	})

	t.Run("parses replacements map", func(t *testing.T) {
		toolsConfig := map[string]any{
			"owner/tool": map[string]any{
				"type":  "github_release",
				"asset": "tool_{{.OS}}_{{.Arch}}.tar.gz",
				"replacements": map[string]any{
					"darwin": "macos",
					"amd64":  "x86_64",
				},
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("owner", "tool")
		require.NoError(t, err)
		require.NotNil(t, tool.Replacements, "replacements should be parsed")
		assert.Equal(t, "macos", tool.Replacements["darwin"])
		assert.Equal(t, "x86_64", tool.Replacements["amd64"])
	})

	t.Run("parses version_prefix", func(t *testing.T) {
		toolsConfig := map[string]any{
			"owner/tool": map[string]any{
				"type":           "github_release",
				"asset":          "tool_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz",
				"version_prefix": "v",
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("owner", "tool")
		require.NoError(t, err)
		assert.Equal(t, "v", tool.VersionPrefix, "version_prefix should be parsed")
	})

	t.Run("parses all optional fields together", func(t *testing.T) {
		// Test the complete replicated-like pattern.
		toolsConfig := map[string]any{
			"replicatedhq/replicated": map[string]any{
				"type":           "github_release",
				"asset":          `replicated_{{trimV .Version}}_{{.OS}}_{{.Arch}}.tar.gz`,
				"format":         "tar.gz",
				"version_prefix": "v",
				"replacements": map[string]any{
					"darwin": "darwin_all",
				},
				"files": []any{
					map[string]any{
						"name": "replicated",
						"src":  "replicated",
					},
				},
				"overrides": []any{
					map[string]any{
						"goos":  "darwin",
						"asset": `replicated_{{trimV .Version}}_darwin_all.tar.gz`,
					},
				},
			},
		}

		reg, err := NewAtmosRegistry(toolsConfig)
		require.NoError(t, err)

		tool, err := reg.GetTool("replicatedhq", "replicated")
		require.NoError(t, err)

		// Verify all fields are parsed.
		assert.Equal(t, "github_release", tool.Type)
		assert.Equal(t, "tar.gz", tool.Format)
		assert.Equal(t, "v", tool.VersionPrefix)
		require.Len(t, tool.Files, 1)
		assert.Equal(t, "replicated", tool.Files[0].Name)
		require.NotNil(t, tool.Replacements)
		assert.Equal(t, "darwin_all", tool.Replacements["darwin"])
		require.Len(t, tool.Overrides, 1)
		assert.Equal(t, "darwin", tool.Overrides[0].GOOS)
	})
}
