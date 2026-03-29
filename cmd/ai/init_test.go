package ai

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInitializeAIToolsAndExecutor_ToolsDisabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Enabled: false,
			},
		},
	}

	toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIToolsDisabled))
	assert.Nil(t, toolsResult)
	// executor is nil when result is nil
}

func TestInitializeAIToolsAndExecutor_ToolsEnabled(t *testing.T) {
	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Enabled:             true,
				AllowedTools:        []string{"read_file"},
				RestrictedTools:     []string{"execute_bash_command"},
				BlockedTools:        []string{"dangerous_tool"},
				YOLOMode:            false,
				RequireConfirmation: boolPtr(true),
			},
		},
	}

	toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

	assert.NoError(t, err)
	assert.NotNil(t, toolsResult)
	assert.NotNil(t, toolsResult.Executor)
	// Registry should have registered tools.
	assert.Greater(t, toolsResult.Registry.Count(), 0)
}

func TestInitializeAIToolsAndExecutor_YOLOMode(t *testing.T) {
	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Enabled:  true,
				YOLOMode: true,
			},
		},
	}

	toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

	assert.NoError(t, err)
	assert.NotNil(t, toolsResult)
	assert.NotNil(t, toolsResult.Executor)
}

func TestInitializeAIToolsAndExecutor_WithToolLists(t *testing.T) {
	basePath := t.TempDir()

	tests := []struct {
		name        string
		toolConfig  schema.AIToolSettings
		shouldError bool
	}{
		{
			name: "with allowed tools only",
			toolConfig: schema.AIToolSettings{
				Enabled:      true,
				AllowedTools: []string{"read_file", "list_files"},
			},
			shouldError: false,
		},
		{
			name: "with restricted tools only",
			toolConfig: schema.AIToolSettings{
				Enabled:         true,
				RestrictedTools: []string{"execute_bash_command"},
			},
			shouldError: false,
		},
		{
			name: "with blocked tools only",
			toolConfig: schema.AIToolSettings{
				Enabled:      true,
				BlockedTools: []string{"dangerous_tool"},
			},
			shouldError: false,
		},
		{
			name: "with all tool lists",
			toolConfig: schema.AIToolSettings{
				Enabled:         true,
				AllowedTools:    []string{"read_file"},
				RestrictedTools: []string{"write_file"},
				BlockedTools:    []string{"delete_file"},
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: basePath,
				AI: schema.AISettings{
					Tools: tt.toolConfig,
				},
			}

			toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, toolsResult)
				assert.NotNil(t, toolsResult.Executor)
			}
		})
	}
}

func TestInitializeAIToolsAndExecutor_RequireConfirmation(t *testing.T) {
	basePath := t.TempDir()

	tests := []struct {
		name                string
		requireConfirmation *bool
	}{
		{
			name:                "require confirmation true",
			requireConfirmation: boolPtr(true),
		},
		{
			name:                "require confirmation false",
			requireConfirmation: boolPtr(false),
		},
		{
			name:                "require confirmation nil (default)",
			requireConfirmation: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: basePath,
				AI: schema.AISettings{
					Tools: schema.AIToolSettings{
						Enabled:             true,
						RequireConfirmation: tt.requireConfirmation,
					},
				},
			}

			toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

			assert.NoError(t, err)
			assert.NotNil(t, toolsResult)
			assert.NotNil(t, toolsResult.Executor)
		})
	}
}

// TestInitializeAIToolsAndExecutor_PermissionCacheFailure tests the permission cache failure path.
// When permission.NewPermissionCache fails, the function should continue without a cache
// and use NewCLIPrompter instead of NewCLIPrompterWithCache.
func TestInitializeAIToolsAndExecutor_PermissionCacheFailure(t *testing.T) {
	// Create a file where the .atmos directory would be created.
	// This will cause os.MkdirAll to fail when trying to create .atmos directory.
	basePath := t.TempDir()

	// Create a file named ".atmos" to block the directory creation.
	atmosFilePath := filepath.Join(basePath, ".atmos")
	err := os.WriteFile(atmosFilePath, []byte("blocking file"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Enabled:             true,
				RequireConfirmation: boolPtr(true),
			},
		},
	}

	// The function should still succeed, but use NewCLIPrompter instead of
	// NewCLIPrompterWithCache due to the permission cache failure.
	toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

	assert.NoError(t, err)
	assert.NotNil(t, toolsResult)
	assert.NotNil(t, toolsResult.Executor)
	assert.Greater(t, toolsResult.Registry.Count(), 0)
}

// TestInitializeAIToolsAndExecutor_EmptyBasePath tests with an empty base path.
// This exercises the permission cache initialization with fallback to home directory.
func TestInitializeAIToolsAndExecutor_EmptyBasePath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "", // Empty base path - cache will use home directory.
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Enabled:  true,
				YOLOMode: true, // Use YOLO mode to simplify testing.
			},
		},
	}

	toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

	assert.NoError(t, err)
	assert.NotNil(t, toolsResult)
	assert.NotNil(t, toolsResult.Executor)
}

// TestInitializeAIToolsAndExecutor_PermissionModes tests different permission mode configurations.
func TestInitializeAIToolsAndExecutor_PermissionModes(t *testing.T) {
	basePath := t.TempDir()

	tests := []struct {
		name                string
		yoloMode            bool
		requireConfirmation *bool
		description         string
	}{
		{
			name:                "YOLO mode - no prompts",
			yoloMode:            true,
			requireConfirmation: nil,
			description:         "YOLO mode bypasses all permission checks",
		},
		{
			name:                "Prompt mode - explicit true",
			yoloMode:            false,
			requireConfirmation: boolPtr(true),
			description:         "Explicit require confirmation",
		},
		{
			name:                "Allow mode - explicit false",
			yoloMode:            false,
			requireConfirmation: boolPtr(false),
			description:         "Opt-out of prompting",
		},
		{
			name:                "Default prompt mode - nil",
			yoloMode:            false,
			requireConfirmation: nil,
			description:         "Default behavior - prompt for security",
		},
		{
			name:                "YOLO takes precedence over require confirmation true",
			yoloMode:            true,
			requireConfirmation: boolPtr(true),
			description:         "YOLO mode overrides RequireConfirmation=true",
		},
		{
			name:                "YOLO takes precedence over require confirmation false",
			yoloMode:            true,
			requireConfirmation: boolPtr(false),
			description:         "YOLO mode overrides RequireConfirmation=false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: basePath,
				AI: schema.AISettings{
					Tools: schema.AIToolSettings{
						Enabled:             true,
						YOLOMode:            tt.yoloMode,
						RequireConfirmation: tt.requireConfirmation,
					},
				},
			}

			toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

			assert.NoError(t, err, tt.description)
			assert.NotNil(t, toolsResult)
			assert.NotNil(t, toolsResult.Executor)
		})
	}
}

// TestInitializeAIToolsAndExecutor_ToolRegistration tests that tools are properly registered.
func TestInitializeAIToolsAndExecutor_ToolRegistration(t *testing.T) {
	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Enabled: true,
			},
		},
	}

	toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

	assert.NoError(t, err)
	assert.NotNil(t, toolsResult)
	assert.NotNil(t, toolsResult.Executor)

	// Verify multiple tools were registered.
	toolCount := toolsResult.Registry.Count()
	assert.Greater(t, toolCount, 5, "Expected more than 5 tools to be registered")
}

// TestInitializeAIToolsAndExecutor_NilPermCacheUsesSimplePrompter tests that when permission
// cache initialization fails, the function uses NewCLIPrompter (without cache).
// This exercises line 54: prompter = permission.NewCLIPrompter().
func TestInitializeAIToolsAndExecutor_NilPermCacheUsesSimplePrompter(t *testing.T) {
	// To trigger the permCache = nil path, we need to make permission.NewPermissionCache fail.
	// One way is to create a file (not directory) at the .atmos path location.
	basePath := t.TempDir()

	// Create .atmos as a file, which will cause the permission cache to fail on mkdir.
	atmosPath := filepath.Join(basePath, ".atmos")
	err := os.WriteFile(atmosPath, []byte("blocking file to fail permission cache"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Enabled:  true,
				YOLOMode: false, // Not YOLO mode - will use prompter
			},
		},
	}

	// The function should still succeed because it handles permCache failure gracefully
	// by using NewCLIPrompter() instead of NewCLIPrompterWithCache().
	toolsResult, err := initializeAIToolsAndExecutor(atmosConfig, nil, "")

	assert.NoError(t, err)
	assert.NotNil(t, toolsResult)
	assert.NotNil(t, toolsResult.Executor)
}

// TestSelectMCPServers tests all branches of the selectMCPServers function.
func TestSelectMCPServers(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{
		"aws":   {Command: "aws-mcp", Description: "AWS tools"},
		"gcp":   {Command: "gcp-mcp", Description: "GCP tools"},
		"azure": {Command: "azure-mcp", Description: "Azure tools"},
	}

	t.Run("manual override with known servers", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			MCP: schema.MCPSettings{
				Servers: servers,
			},
		}
		result := selectMCPServers(atmosConfig, []string{"aws", "gcp"}, "some question")
		assert.Len(t, result, 2)
		assert.Contains(t, result, "aws")
		assert.Contains(t, result, "gcp")
		assert.NotContains(t, result, "azure")
	})

	t.Run("manual override with unknown server name", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			MCP: schema.MCPSettings{
				Servers: servers,
			},
		}
		// "nonexistent" is not in the servers map - should still return any valid ones.
		result := selectMCPServers(atmosConfig, []string{"aws", "nonexistent"}, "")
		assert.Len(t, result, 1)
		assert.Contains(t, result, "aws")
	})

	t.Run("manual override all unknown returns empty", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			MCP: schema.MCPSettings{
				Servers: servers,
			},
		}
		result := selectMCPServers(atmosConfig, []string{"nonexistent1", "nonexistent2"}, "")
		assert.Empty(t, result)
	})

	t.Run("single server no routing needed", func(t *testing.T) {
		singleServer := map[string]schema.MCPServerConfig{
			"only": {Command: "only-mcp"},
		}
		atmosConfig := &schema.AtmosConfiguration{
			MCP: schema.MCPSettings{
				Servers: singleServer,
			},
		}
		result := selectMCPServers(atmosConfig, nil, "some question")
		assert.Len(t, result, 1)
		assert.Contains(t, result, "only")
	})

	t.Run("routing disabled returns all servers", func(t *testing.T) {
		routingDisabled := false
		atmosConfig := &schema.AtmosConfiguration{
			MCP: schema.MCPSettings{
				Servers: servers,
				Routing: schema.MCPRoutingConfig{
					Enabled: &routingDisabled,
				},
			},
		}
		result := selectMCPServers(atmosConfig, nil, "deploy to AWS")
		assert.Len(t, result, 3)
		assert.Equal(t, servers, result)
	})

	t.Run("empty question returns all servers", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			MCP: schema.MCPSettings{
				Servers: servers,
			},
		}
		// Empty question = chat mode, returns all servers.
		result := selectMCPServers(atmosConfig, nil, "")
		assert.Len(t, result, 3)
		assert.Equal(t, servers, result)
	})

	t.Run("multiple servers with empty question and routing enabled", func(t *testing.T) {
		routingEnabled := true
		atmosConfig := &schema.AtmosConfiguration{
			MCP: schema.MCPSettings{
				Servers: servers,
				Routing: schema.MCPRoutingConfig{
					Enabled: &routingEnabled,
				},
			},
		}
		// Even with routing enabled, empty question returns all.
		result := selectMCPServers(atmosConfig, nil, "")
		assert.Len(t, result, 3)
	})
}

// TestFilterServersByName tests the filterServersByName function.
func TestFilterServersByName(t *testing.T) {
	servers := map[string]schema.MCPServerConfig{
		"aws":   {Command: "aws-mcp"},
		"gcp":   {Command: "gcp-mcp"},
		"azure": {Command: "azure-mcp"},
	}

	t.Run("all names found", func(t *testing.T) {
		result := filterServersByName(servers, []string{"aws", "gcp", "azure"})
		assert.Len(t, result, 3)
		assert.Equal(t, "aws-mcp", result["aws"].Command)
		assert.Equal(t, "gcp-mcp", result["gcp"].Command)
		assert.Equal(t, "azure-mcp", result["azure"].Command)
	})

	t.Run("some names not found", func(t *testing.T) {
		result := filterServersByName(servers, []string{"aws", "nonexistent"})
		assert.Len(t, result, 1)
		assert.Contains(t, result, "aws")
		assert.NotContains(t, result, "nonexistent")
	})

	t.Run("empty names list", func(t *testing.T) {
		result := filterServersByName(servers, []string{})
		assert.Empty(t, result)
	})

	t.Run("empty servers map", func(t *testing.T) {
		result := filterServersByName(map[string]schema.MCPServerConfig{}, []string{"aws"})
		assert.Empty(t, result)
	})

	t.Run("nil servers map", func(t *testing.T) {
		result := filterServersByName(nil, []string{"aws"})
		assert.Empty(t, result)
	})
}

// TestSortedServerNames tests the sortedServerNames function.
func TestSortedServerNames(t *testing.T) {
	t.Run("multiple servers returned sorted", func(t *testing.T) {
		servers := map[string]schema.MCPServerConfig{
			"zebra":  {Command: "z"},
			"apple":  {Command: "a"},
			"mango":  {Command: "m"},
			"banana": {Command: "b"},
		}
		result := sortedServerNames(servers)
		assert.Equal(t, []string{"apple", "banana", "mango", "zebra"}, result)
	})

	t.Run("empty map", func(t *testing.T) {
		result := sortedServerNames(map[string]schema.MCPServerConfig{})
		assert.Empty(t, result)
	})

	t.Run("single server", func(t *testing.T) {
		servers := map[string]schema.MCPServerConfig{
			"only": {Command: "only-mcp"},
		}
		result := sortedServerNames(servers)
		assert.Equal(t, []string{"only"}, result)
	})
}

// TestInitializeAIReadOnlyTools_ToolsDisabled tests that initializeAIReadOnlyTools
// returns nil and ErrAIToolsDisabled when tools are disabled.
func TestInitializeAIReadOnlyTools_ToolsDisabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Enabled: false,
			},
		},
	}

	result, err := initializeAIReadOnlyTools(atmosConfig, nil, "")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIToolsDisabled))
	assert.Nil(t, result)
}

// TestInitializeAIReadOnlyTools_ToolsEnabled tests that initializeAIReadOnlyTools
// returns a valid result with registered tools when tools are enabled.
func TestInitializeAIReadOnlyTools_ToolsEnabled(t *testing.T) {
	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Enabled: true,
			},
		},
	}

	result, err := initializeAIReadOnlyTools(atmosConfig, nil, "")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Executor)
	assert.NotNil(t, result.Registry)
	assert.Greater(t, result.Registry.Count(), 0, "Expected at least one read-only tool to be registered")
}

// TestServersNeedAuth tests the serversNeedAuth function.
func TestServersNeedAuth(t *testing.T) {
	t.Run("no servers need auth", func(t *testing.T) {
		servers := map[string]schema.MCPServerConfig{
			"aws": {Command: "aws-mcp"},
			"gcp": {Command: "gcp-mcp"},
		}
		assert.False(t, serversNeedAuth(servers))
	})

	t.Run("one server needs auth", func(t *testing.T) {
		servers := map[string]schema.MCPServerConfig{
			"aws": {Command: "aws-mcp", Identity: "aws-identity"},
			"gcp": {Command: "gcp-mcp"},
		}
		assert.True(t, serversNeedAuth(servers))
	})

	t.Run("all servers need auth", func(t *testing.T) {
		servers := map[string]schema.MCPServerConfig{
			"aws": {Command: "aws-mcp", Identity: "aws-id"},
			"gcp": {Command: "gcp-mcp", Identity: "gcp-id"},
		}
		assert.True(t, serversNeedAuth(servers))
	})

	t.Run("empty map", func(t *testing.T) {
		assert.False(t, serversNeedAuth(map[string]schema.MCPServerConfig{}))
	})

	t.Run("nil map", func(t *testing.T) {
		assert.False(t, serversNeedAuth(nil))
	})
}
