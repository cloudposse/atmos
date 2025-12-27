package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
