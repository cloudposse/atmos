package registry

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/toolchain"
	toolchainregistry "github.com/cloudposse/atmos/toolchain/registry"
)

// TestValidateSearchFormat tests validateSearchFormat function.
func TestValidateSearchFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{
			name:    "table format is valid",
			format:  "table",
			wantErr: false,
		},
		{
			name:    "json format is valid",
			format:  "json",
			wantErr: false,
		},
		{
			name:    "yaml format is valid",
			format:  "yaml",
			wantErr: false,
		},
		{
			name:    "xml format is invalid",
			format:  "xml",
			wantErr: true,
		},
		{
			name:    "csv format is invalid",
			format:  "csv",
			wantErr: true,
		},
		{
			name:    "empty format is invalid",
			format:  "",
			wantErr: true,
		},
		{
			name:    "random string is invalid",
			format:  "random",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSearchFormat(tt.format)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateSearchRegistry tests createSearchRegistry function.
func TestCreateSearchRegistry(t *testing.T) {
	tests := []struct {
		name         string
		registryName string
		wantErr      bool
		errIs        error
	}{
		{
			name:         "empty string uses default",
			registryName: "",
			wantErr:      false,
		},
		{
			name:         "aqua-public is valid",
			registryName: "aqua-public",
			wantErr:      false,
		},
		{
			name:         "aqua is valid",
			registryName: "aqua",
			wantErr:      false,
		},
		{
			name:         "unknown registry returns error",
			registryName: "unknown-registry",
			wantErr:      true,
			errIs:        toolchainregistry.ErrUnknownRegistry,
		},
		{
			name:         "custom registry returns error",
			registryName: "my-custom-registry",
			wantErr:      true,
			errIs:        toolchainregistry.ErrUnknownRegistry,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, err := createSearchRegistry(tt.registryName)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				assert.Nil(t, reg)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, reg)
			}
		})
	}
}

// TestSearchRow tests searchRow struct.
func TestSearchRow(t *testing.T) {
	t.Run("creates search row with all fields", func(t *testing.T) {
		row := searchRow{
			status:      statusIndicator,
			toolName:    "hashicorp/terraform",
			toolType:    "github_release",
			registry:    "aqua-public",
			isInstalled: true,
			isInConfig:  true,
		}
		assert.Equal(t, statusIndicator, row.status)
		assert.Equal(t, "hashicorp/terraform", row.toolName)
		assert.Equal(t, "github_release", row.toolType)
		assert.Equal(t, "aqua-public", row.registry)
		assert.True(t, row.isInstalled)
		assert.True(t, row.isInConfig)
	})

	t.Run("creates search row without status", func(t *testing.T) {
		row := searchRow{
			status:      " ",
			toolName:    "kubernetes/kubectl",
			toolType:    "github_release",
			registry:    "aqua-public",
			isInstalled: false,
			isInConfig:  false,
		}
		assert.Equal(t, " ", row.status)
		assert.False(t, row.isInstalled)
		assert.False(t, row.isInConfig)
	})
}

// TestSearchConstants tests that search constants are defined correctly.
func TestSearchConstants(t *testing.T) {
	assert.Equal(t, 20, defaultSearchLimit)
	assert.Equal(t, 30, columnWidthTool)
	assert.Equal(t, 15, columnWidthToolType)
	assert.Equal(t, 20, columnWidthToolRegistry)
}

// TestGetSearchParser tests GetSearchParser function.
func TestGetSearchParser(t *testing.T) {
	parser := GetSearchParser()
	require.NotNil(t, parser)
	assert.Equal(t, searchParser, parser)
}

// TestDisplaySearchTable tests displaySearchTable function.
func TestDisplaySearchTable(t *testing.T) {
	tests := []struct {
		name  string
		tools []*toolchainregistry.Tool
		query string
		limit int
	}{
		{
			name:  "empty results",
			tools: []*toolchainregistry.Tool{},
			query: "nonexistent",
			limit: 20,
		},
		{
			name: "results within limit",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "hashicorp",
					RepoName:  "terraform",
					Type:      "github_release",
					Registry:  "aqua-public",
				},
			},
			query: "terraform",
			limit: 20,
		},
		{
			name: "results exceed limit",
			tools: []*toolchainregistry.Tool{
				{RepoOwner: "owner1", RepoName: "repo1", Type: "github_release", Registry: "aqua"},
				{RepoOwner: "owner2", RepoName: "repo2", Type: "github_release", Registry: "aqua"},
				{RepoOwner: "owner3", RepoName: "repo3", Type: "github_release", Registry: "aqua"},
			},
			query: "repo",
			limit: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic.
			assert.NotPanics(t, func() {
				displaySearchTable(tt.tools, tt.query, tt.limit)
			})
		})
	}
}

// TestDisplaySearchResults_VariousInputs tests displaySearchResults with various inputs.
func TestDisplaySearchResults_VariousInputs(t *testing.T) {
	tests := []struct {
		name  string
		tools []*toolchainregistry.Tool
	}{
		{
			name:  "nil tools",
			tools: nil,
		},
		{
			name:  "empty slice",
			tools: []*toolchainregistry.Tool{},
		},
		{
			name: "tool with empty registry",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "owner",
					RepoName:  "repo",
					Type:      "github_release",
					Registry:  "",
				},
			},
		},
		{
			name: "tool with long names",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "very-long-organization-name-here",
					RepoName:  "very-long-repository-name-that-exceeds-normal-length",
					Type:      "github_release",
					Registry:  "aqua-public",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				displaySearchResults(tt.tools)
			})
		})
	}
}

// TestParseSearchFlags tests parseSearchFlags function.
func TestParseSearchFlags(t *testing.T) {
	tests := []struct {
		name          string
		limit         int
		registry      string
		installedOnly bool
		availableOnly bool
		format        string
	}{
		{
			name:          "default values",
			limit:         20,
			registry:      "",
			installedOnly: false,
			availableOnly: false,
			format:        "table",
		},
		{
			name:          "custom limit and registry",
			limit:         50,
			registry:      "aqua",
			installedOnly: true,
			availableOnly: false,
			format:        "json",
		},
		{
			name:          "uppercase format normalized",
			limit:         10,
			registry:      "",
			installedOnly: false,
			availableOnly: true,
			format:        "YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.Set("limit", tt.limit)
			v.Set("registry", tt.registry)
			v.Set("installed-only", tt.installedOnly)
			v.Set("available-only", tt.availableOnly)
			v.Set("format", tt.format)

			flags := parseSearchFlags(v)

			assert.Equal(t, tt.limit, flags.limit)
			assert.Equal(t, tt.registry, flags.registry)
			assert.Equal(t, tt.installedOnly, flags.installedOnly)
			assert.Equal(t, tt.availableOnly, flags.availableOnly)
			// Format should be lowercased.
			assert.Equal(t, strings.ToLower(tt.format), flags.format)
		})
	}
}

// TestValidateSearchFlags tests validateSearchFlags function.
func TestValidateSearchFlags(t *testing.T) {
	tests := []struct {
		name        string
		flags       searchFlags
		wantErr     bool
		errContains string
	}{
		{
			name: "valid flags",
			flags: searchFlags{
				limit:         20,
				registry:      "",
				installedOnly: false,
				availableOnly: false,
				format:        "table",
			},
			wantErr: false,
		},
		{
			name: "invalid format",
			flags: searchFlags{
				limit:         20,
				registry:      "",
				installedOnly: false,
				availableOnly: false,
				format:        "xml",
			},
			wantErr:     true,
			errContains: "format must be one of",
		},
		{
			name: "negative limit",
			flags: searchFlags{
				limit:         -1,
				registry:      "",
				installedOnly: false,
				availableOnly: false,
				format:        "table",
			},
			wantErr:     true,
			errContains: "limit must be non-negative",
		},
		{
			name: "both installed-only and available-only",
			flags: searchFlags{
				limit:         20,
				registry:      "",
				installedOnly: true,
				availableOnly: true,
				format:        "table",
			},
			wantErr:     true,
			errContains: "cannot use both",
		},
		{
			name: "zero limit is valid",
			flags: searchFlags{
				limit:         0,
				registry:      "",
				installedOnly: false,
				availableOnly: false,
				format:        "table",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSearchFlags(tt.flags)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGetSearchStatusIndicator tests getSearchStatusIndicator function.
func TestGetSearchStatusIndicator(t *testing.T) {
	tests := []struct {
		name        string
		isInstalled bool
		isInConfig  bool
		want        string
	}{
		{
			name:        "installed returns dot",
			isInstalled: true,
			isInConfig:  true,
			want:        statusIndicator,
		},
		{
			name:        "in config but not installed returns dot",
			isInstalled: false,
			isInConfig:  true,
			want:        statusIndicator,
		},
		{
			name:        "not in config returns space",
			isInstalled: false,
			isInConfig:  false,
			want:        " ",
		},
		{
			name:        "installed but not in config returns dot",
			isInstalled: true,
			isInConfig:  false,
			want:        statusIndicator,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSearchStatusIndicator(tt.isInstalled, tt.isInConfig)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestUpdateSearchColumnWidths tests updateSearchColumnWidths function.
func TestUpdateSearchColumnWidths(t *testing.T) {
	tests := []struct {
		name         string
		initialWidth searchColumnWidths
		tool         *toolchainregistry.Tool
		toolName     string
		wantToolName int
		wantToolType int
		wantRegistry int
	}{
		{
			name: "updates toolName width when longer",
			initialWidth: searchColumnWidths{
				toolName: 10,
				toolType: 10,
				registry: 10,
			},
			tool:         &toolchainregistry.Tool{Type: "http", Registry: "aqua"},
			toolName:     "very-long-tool-name-here",
			wantToolName: 24, // Matches length of toolName string above.
			wantToolType: 10,
			wantRegistry: 10,
		},
		{
			name: "updates toolType width when longer",
			initialWidth: searchColumnWidths{
				toolName: 10,
				toolType: 5,
				registry: 10,
			},
			tool:         &toolchainregistry.Tool{Type: "github_release", Registry: "aqua"},
			toolName:     "short",
			wantToolName: 10,
			wantToolType: 14, // Matches length of Type field above.
			wantRegistry: 10,
		},
		{
			name: "updates registry width when longer",
			initialWidth: searchColumnWidths{
				toolName: 10,
				toolType: 10,
				registry: 5,
			},
			tool:         &toolchainregistry.Tool{Type: "http", Registry: "aqua-public"},
			toolName:     "short",
			wantToolName: 10,
			wantToolType: 10,
			wantRegistry: 11, // Matches length of Registry field above.
		},
		{
			name: "keeps widths when tool is shorter",
			initialWidth: searchColumnWidths{
				toolName: 30,
				toolType: 20,
				registry: 20,
			},
			tool:         &toolchainregistry.Tool{Type: "http", Registry: "aqua"},
			toolName:     "short",
			wantToolName: 30,
			wantToolType: 20,
			wantRegistry: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateSearchColumnWidths(tt.initialWidth, tt.tool, tt.toolName)
			assert.Equal(t, tt.wantToolName, got.toolName)
			assert.Equal(t, tt.wantToolType, got.toolType)
			assert.Equal(t, tt.wantRegistry, got.registry)
		})
	}
}

// TestGetSearchToolVersion tests getSearchToolVersion function.
func TestGetSearchToolVersion(t *testing.T) {
	tests := []struct {
		name      string
		fullName  string
		repoName  string
		tools     map[string][]string
		foundFull bool
		foundRepo bool
		want      string
	}{
		{
			name:      "returns version from full name",
			fullName:  "hashicorp/terraform",
			repoName:  "terraform",
			tools:     map[string][]string{"hashicorp/terraform": {"1.5.0", "1.4.0"}},
			foundFull: true,
			foundRepo: false,
			want:      "1.5.0",
		},
		{
			name:      "returns version from repo name when full not found",
			fullName:  "hashicorp/terraform",
			repoName:  "terraform",
			tools:     map[string][]string{"terraform": {"1.5.0"}},
			foundFull: false,
			foundRepo: true,
			want:      "1.5.0",
		},
		{
			name:      "returns empty when no versions for full name",
			fullName:  "hashicorp/terraform",
			repoName:  "terraform",
			tools:     map[string][]string{"hashicorp/terraform": {}},
			foundFull: true,
			foundRepo: false,
			want:      "",
		},
		{
			name:      "returns empty when no versions for repo name",
			fullName:  "hashicorp/terraform",
			repoName:  "terraform",
			tools:     map[string][]string{"terraform": {}},
			foundFull: false,
			foundRepo: true,
			want:      "",
		},
		{
			name:      "returns empty when not found",
			fullName:  "hashicorp/terraform",
			repoName:  "terraform",
			tools:     map[string][]string{},
			foundFull: false,
			foundRepo: false,
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolVersions := &toolchain.ToolVersions{
				Tools: tt.tools,
			}
			got := getSearchToolVersion(tt.fullName, tt.repoName, toolVersions, tt.foundFull, tt.foundRepo)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildSearchRows tests buildSearchRows function.
func TestBuildSearchRows(t *testing.T) {
	tools := []*toolchainregistry.Tool{
		{
			RepoOwner: "hashicorp",
			RepoName:  "terraform",
			Type:      "github_release",
			Registry:  "aqua-public",
		},
		{
			RepoOwner: "kubernetes",
			RepoName:  "kubectl",
			Type:      "github_release",
			Registry:  "aqua-public",
		},
	}

	// Test with nil toolVersions.
	rows, widths := buildSearchRows(tools, nil, nil)

	assert.Len(t, rows, 2)
	assert.Equal(t, "hashicorp/terraform", rows[0].toolName)
	assert.Equal(t, "github_release", rows[0].toolType)
	assert.Equal(t, "aqua-public", rows[0].registry)
	assert.Equal(t, "kubernetes/kubectl", rows[1].toolName)

	// Check widths were calculated.
	assert.GreaterOrEqual(t, widths.toolName, len("hashicorp/terraform"))
	assert.GreaterOrEqual(t, widths.toolType, len("github_release"))
	assert.GreaterOrEqual(t, widths.registry, len("aqua-public"))
}

// TestBuildSingleSearchRow tests buildSingleSearchRow function.
func TestBuildSingleSearchRow(t *testing.T) {
	tool := &toolchainregistry.Tool{
		RepoOwner: "hashicorp",
		RepoName:  "terraform",
		Type:      "github_release",
		Registry:  "aqua-public",
	}

	// Test with nil toolVersions.
	row := buildSingleSearchRow(tool, nil, nil)

	assert.Equal(t, "hashicorp/terraform", row.toolName)
	assert.Equal(t, "github_release", row.toolType)
	assert.Equal(t, "aqua-public", row.registry)
	assert.False(t, row.isInstalled)
	assert.False(t, row.isInConfig)
	assert.Equal(t, " ", row.status) // Not in config, should be space.
}

// TestRenderSearchTable tests renderSearchTable function.
func TestRenderSearchTable(t *testing.T) {
	rows := []searchRow{
		{
			status:      statusIndicator,
			toolName:    "hashicorp/terraform",
			toolType:    "github_release",
			registry:    "aqua-public",
			isInstalled: true,
			isInConfig:  true,
		},
		{
			status:      " ",
			toolName:    "kubernetes/kubectl",
			toolType:    "github_release",
			registry:    "aqua-public",
			isInstalled: false,
			isInConfig:  false,
		},
	}

	widths := searchColumnWidths{
		status:   3,
		toolName: 25,
		toolType: 15,
		registry: 15,
	}

	result := renderSearchTable(rows, widths)

	// Should contain table output.
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "TOOL")
	assert.Contains(t, result, "TYPE")
	assert.Contains(t, result, "REGISTRY")
	assert.Contains(t, result, "hashicorp/terraform")
	assert.Contains(t, result, "kubernetes/kubectl")
}

// TestSearchColumnWidths tests searchColumnWidths struct.
func TestSearchColumnWidths(t *testing.T) {
	widths := searchColumnWidths{
		status:   1,
		toolName: 30,
		toolType: 15,
		registry: 20,
	}

	assert.Equal(t, 1, widths.status)
	assert.Equal(t, 30, widths.toolName)
	assert.Equal(t, 15, widths.toolType)
	assert.Equal(t, 20, widths.registry)
}

// TestSearchFlags tests searchFlags struct.
func TestSearchFlags(t *testing.T) {
	flags := searchFlags{
		limit:         50,
		registry:      "aqua",
		installedOnly: true,
		availableOnly: false,
		format:        "json",
	}

	assert.Equal(t, 50, flags.limit)
	assert.Equal(t, "aqua", flags.registry)
	assert.True(t, flags.installedOnly)
	assert.False(t, flags.availableOnly)
	assert.Equal(t, "json", flags.format)
}

// TestCheckSearchToolStatus tests the checkSearchToolStatus function.
func TestCheckSearchToolStatus(t *testing.T) {
	tests := getToolStatusTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer := toolchain.NewInstaller()
			inConfig, installed := checkSearchToolStatus(tt.tool, tt.toolVersions, installer)
			assert.Equal(t, tt.wantInConfig, inConfig)
			assert.Equal(t, tt.wantInstall, installed)
		})
	}
}

// TestBuildSingleSearchRow_WithToolVersions tests buildSingleSearchRow with actual toolVersions.
func TestBuildSingleSearchRow_WithToolVersions(t *testing.T) {
	tests := []struct {
		name         string
		tool         *toolchainregistry.Tool
		toolVersions *toolchain.ToolVersions
		wantInConfig bool
	}{
		{
			name: "tool in config by full name",
			tool: &toolchainregistry.Tool{
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Type:      "github_release",
				Registry:  "aqua-public",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"hashicorp/terraform": {"1.5.0"},
				},
			},
			wantInConfig: true,
		},
		{
			name: "tool in config by repo name",
			tool: &toolchainregistry.Tool{
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Type:      "github_release",
				Registry:  "aqua-public",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
				},
			},
			wantInConfig: true,
		},
		{
			name: "tool not in config",
			tool: &toolchainregistry.Tool{
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Type:      "github_release",
				Registry:  "aqua-public",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"other/tool": {"1.0.0"},
				},
			},
			wantInConfig: false,
		},
		{
			name: "nil tools map",
			tool: &toolchainregistry.Tool{
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Type:      "github_release",
				Registry:  "aqua-public",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: nil,
			},
			wantInConfig: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer := toolchain.NewInstaller()
			row := buildSingleSearchRow(tt.tool, tt.toolVersions, installer)

			assert.Equal(t, "hashicorp/terraform", row.toolName)
			assert.Equal(t, tt.tool.Type, row.toolType)
			assert.Equal(t, tt.tool.Registry, row.registry)
			assert.Equal(t, tt.wantInConfig, row.isInConfig)
		})
	}
}

// TestBuildSearchRows_WithToolVersions tests buildSearchRows with actual toolVersions.
func TestBuildSearchRows_WithToolVersions(t *testing.T) {
	tools := []*toolchainregistry.Tool{
		{
			RepoOwner: "hashicorp",
			RepoName:  "terraform",
			Type:      "github_release",
			Registry:  "aqua-public",
		},
		{
			RepoOwner: "kubernetes",
			RepoName:  "kubectl",
			Type:      "github_release",
			Registry:  "aqua-public",
		},
	}

	toolVersions := &toolchain.ToolVersions{
		Tools: map[string][]string{
			"hashicorp/terraform": {"1.5.0"},
		},
	}

	installer := toolchain.NewInstaller()
	rows, widths := buildSearchRows(tools, toolVersions, installer)

	assert.Len(t, rows, 2)
	assert.True(t, rows[0].isInConfig, "terraform should be in config")
	assert.False(t, rows[1].isInConfig, "kubectl should not be in config")
	assert.GreaterOrEqual(t, widths.toolName, len("hashicorp/terraform"))
}

// TestSearchCommand_DefaultFlagValues tests default values for search command flags.
func TestSearchCommand_DefaultFlagValues(t *testing.T) {
	t.Run("limit default is 20", func(t *testing.T) {
		flag := searchCmd.Flags().Lookup("limit")
		require.NotNil(t, flag)
		assert.Equal(t, "20", flag.DefValue)
	})

	t.Run("registry default is empty", func(t *testing.T) {
		flag := searchCmd.Flags().Lookup("registry")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("format default is table", func(t *testing.T) {
		flag := searchCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "table", flag.DefValue)
	})

	t.Run("installed-only default is false", func(t *testing.T) {
		flag := searchCmd.Flags().Lookup("installed-only")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("available-only default is false", func(t *testing.T) {
		flag := searchCmd.Flags().Lookup("available-only")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

// TestSearchCommand_CommandStructure tests the search command structure.
func TestSearchCommand_CommandStructure(t *testing.T) {
	t.Run("command has correct use string", func(t *testing.T) {
		assert.Contains(t, searchCmd.Use, "search")
	})

	t.Run("command has short description", func(t *testing.T) {
		assert.NotEmpty(t, searchCmd.Short)
	})

	t.Run("command has long description", func(t *testing.T) {
		assert.NotEmpty(t, searchCmd.Long)
		assert.Contains(t, searchCmd.Long, "query")
	})

	t.Run("command has RunE function", func(t *testing.T) {
		assert.NotNil(t, searchCmd.RunE)
	})

	t.Run("command requires exactly 1 argument", func(t *testing.T) {
		assert.NotNil(t, searchCmd.Args)
	})

	t.Run("command silences usage on error", func(t *testing.T) {
		assert.True(t, searchCmd.SilenceUsage)
	})

	t.Run("command silences errors", func(t *testing.T) {
		assert.True(t, searchCmd.SilenceErrors)
	})
}

// TestDisplaySearchTable_EdgeCases tests displaySearchTable with edge cases.
func TestDisplaySearchTable_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		tools []*toolchainregistry.Tool
		query string
		limit int
	}{
		{
			name:  "zero limit shows all",
			tools: make([]*toolchainregistry.Tool, 5),
			query: "test",
			limit: 0,
		},
		{
			name:  "negative limit shows all",
			tools: make([]*toolchainregistry.Tool, 5),
			query: "test",
			limit: -1,
		},
		{
			name:  "limit equal to results",
			tools: make([]*toolchainregistry.Tool, 5),
			query: "test",
			limit: 5,
		},
		{
			name:  "limit greater than results",
			tools: make([]*toolchainregistry.Tool, 5),
			query: "test",
			limit: 10,
		},
	}

	// Initialize tools with valid data.
	for i := range tests {
		for j := range tests[i].tools {
			tests[i].tools[j] = &toolchainregistry.Tool{
				RepoOwner: "owner",
				RepoName:  "repo",
				Type:      "github_release",
				Registry:  "aqua",
			}
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic.
			assert.NotPanics(t, func() {
				displaySearchTable(tt.tools, tt.query, tt.limit)
			})
		})
	}
}

// TestOutputSearchResults_AllFormats tests outputSearchResults with all formats.
func TestOutputSearchResults_AllFormats(t *testing.T) {
	tools := []*toolchainregistry.Tool{
		{
			RepoOwner: "hashicorp",
			RepoName:  "terraform",
			Type:      "github_release",
			Registry:  "aqua-public",
		},
	}

	tests := []struct {
		name   string
		flags  searchFlags
		tools  []*toolchainregistry.Tool
		query  string
		panics bool
	}{
		{
			name:  "table format",
			flags: searchFlags{format: "table", limit: 10},
			tools: tools,
			query: "terraform",
		},
		{
			name:  "empty results",
			flags: searchFlags{format: "table", limit: 10},
			tools: []*toolchainregistry.Tool{},
			query: "nonexistent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				_ = outputSearchResults(tt.tools, tt.query, tt.flags)
			})
		})
	}
}
