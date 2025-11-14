package azure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestNewConsoleURLGenerator(t *testing.T) {
	generator := NewConsoleURLGenerator()
	assert.NotNil(t, generator)
	assert.True(t, generator.SupportsConsoleAccess())
}

func TestConsoleURLGenerator_SupportsConsoleAccess(t *testing.T) {
	generator := &ConsoleURLGenerator{}
	assert.True(t, generator.SupportsConsoleAccess())
}

func TestConsoleURLGenerator_GetConsoleURL(t *testing.T) {
	tests := []struct {
		name            string
		creds           types.ICredentials
		options         types.ConsoleURLOptions
		expectedURL     string
		expectedContain string
		expectError     bool
		errorType       error
	}{
		{
			name: "default home destination",
			creds: &types.AzureCredentials{
				AccessToken:    "test-token",
				TenantID:       "tenant-123",
				SubscriptionID: "sub-456",
			},
			options:     types.ConsoleURLOptions{},
			expectedURL: "https://portal.azure.com/#@tenant-123",
			expectError: false,
		},
		{
			name: "subscription destination",
			creds: &types.AzureCredentials{
				AccessToken:    "test-token",
				TenantID:       "tenant-123",
				SubscriptionID: "sub-456",
			},
			options: types.ConsoleURLOptions{
				Destination: "subscription",
			},
			expectedContain: "/resource/subscriptions/sub-456/overview",
			expectError:     false,
		},
		{
			name: "resource groups destination",
			creds: &types.AzureCredentials{
				AccessToken:    "test-token",
				TenantID:       "tenant-123",
				SubscriptionID: "sub-456",
			},
			options: types.ConsoleURLOptions{
				Destination: "resourcegroups",
			},
			expectedContain: "/blade/HubsExtension/BrowseResourceGroups",
			expectError:     false,
		},
		{
			name: "custom session duration",
			creds: &types.AzureCredentials{
				AccessToken:    "test-token",
				TenantID:       "tenant-123",
				SubscriptionID: "sub-456",
			},
			options: types.ConsoleURLOptions{
				SessionDuration: 2 * time.Hour,
			},
			expectedURL: "https://portal.azure.com/#@tenant-123",
			expectError: false,
		},
		{
			name: "full URL destination",
			creds: &types.AzureCredentials{
				AccessToken:    "test-token",
				TenantID:       "tenant-123",
				SubscriptionID: "sub-456",
			},
			options: types.ConsoleURLOptions{
				Destination: "https://portal.azure.com/#custom/path",
			},
			expectedURL: "https://portal.azure.com/#custom/path",
			expectError: false,
		},
		{
			name: "missing access token",
			creds: &types.AzureCredentials{
				TenantID: "tenant-123",
			},
			options:     types.ConsoleURLOptions{},
			expectError: true,
			errorType:   errUtils.ErrInvalidAuthConfig,
		},
		{
			name: "missing tenant ID",
			creds: &types.AzureCredentials{
				AccessToken: "test-token",
			},
			options:     types.ConsoleURLOptions{},
			expectError: true,
			errorType:   errUtils.ErrInvalidAuthConfig,
		},
		{
			name:        "wrong credential type",
			creds:       &types.AWSCredentials{},
			options:     types.ConsoleURLOptions{},
			expectError: true,
			errorType:   errUtils.ErrInvalidAuthConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := NewConsoleURLGenerator()
			ctx := context.Background()

			url, duration, err := generator.GetConsoleURL(ctx, tt.creds, tt.options)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorType != nil {
					assert.True(t, errors.Is(err, tt.errorType))
				}
				return
			}

			require.NoError(t, err)
			if tt.expectedURL != "" {
				assert.Equal(t, tt.expectedURL, url)
			}
			if tt.expectedContain != "" {
				assert.Contains(t, url, tt.expectedContain)
			}

			// Check duration.
			if tt.options.SessionDuration != 0 {
				assert.Equal(t, tt.options.SessionDuration, duration)
			} else {
				assert.Equal(t, AzureDefaultSessionDuration, duration)
			}
		})
	}
}

func TestValidateAzureCredentials(t *testing.T) {
	tests := []struct {
		name        string
		creds       types.ICredentials
		expectError bool
		errorType   error
	}{
		{
			name: "valid Azure credentials",
			creds: &types.AzureCredentials{
				AccessToken: "test-token",
				TenantID:    "tenant-123",
			},
			expectError: false,
		},
		{
			name: "missing access token",
			creds: &types.AzureCredentials{
				TenantID: "tenant-123",
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidAuthConfig,
		},
		{
			name: "missing tenant ID",
			creds: &types.AzureCredentials{
				AccessToken: "test-token",
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidAuthConfig,
		},
		{
			name:        "wrong credential type - AWS",
			creds:       &types.AWSCredentials{},
			expectError: true,
			errorType:   errUtils.ErrInvalidAuthConfig,
		},
		{
			name:        "nil credentials",
			creds:       nil,
			expectError: true,
			errorType:   errUtils.ErrInvalidAuthConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateAzureCredentials(tt.creds)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, result)
				if tt.errorType != nil {
					assert.True(t, errors.Is(err, tt.errorType))
				}
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.creds.(*types.AzureCredentials).AccessToken, result.AccessToken)
			assert.Equal(t, tt.creds.(*types.AzureCredentials).TenantID, result.TenantID)
		})
	}
}

func TestDetermineSessionDuration(t *testing.T) {
	tests := []struct {
		name      string
		requested time.Duration
		expected  time.Duration
	}{
		{
			name:      "zero duration uses default",
			requested: 0,
			expected:  AzureDefaultSessionDuration,
		},
		{
			name:      "custom duration is preserved",
			requested: 2 * time.Hour,
			expected:  2 * time.Hour,
		},
		{
			name:      "30 minute duration",
			requested: 30 * time.Minute,
			expected:  30 * time.Minute,
		},
		{
			name:      "12 hour duration",
			requested: 12 * time.Hour,
			expected:  12 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineSessionDuration(tt.requested)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveDestinationWithDefault(t *testing.T) {
	creds := &types.AzureCredentials{
		TenantID:       "tenant-123",
		SubscriptionID: "sub-456",
	}

	tests := []struct {
		name            string
		dest            string
		creds           *types.AzureCredentials
		expectedContain string
		expectError     bool
	}{
		{
			name:            "empty destination uses default",
			dest:            "",
			creds:           creds,
			expectedContain: "#@tenant-123",
			expectError:     false,
		},
		{
			name:            "home destination",
			dest:            "home",
			creds:           creds,
			expectedContain: "#@tenant-123",
			expectError:     false,
		},
		{
			name:            "subscription destination",
			dest:            "subscription",
			creds:           creds,
			expectedContain: "/subscriptions/sub-456",
			expectError:     false,
		},
		{
			name:        "subscription without ID fails",
			dest:        "subscription",
			creds:       &types.AzureCredentials{TenantID: "tenant-123"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveDestinationWithDefault(tt.dest, tt.creds)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.expectedContain != "" {
				assert.Contains(t, result, tt.expectedContain)
			}
			// Should never return empty string.
			assert.NotEmpty(t, result)
		})
	}
}

func TestResolveDestination(t *testing.T) {
	creds := &types.AzureCredentials{
		TenantID:       "tenant-123",
		SubscriptionID: "sub-456",
	}

	tests := []struct {
		name            string
		dest            string
		expectedContain string
		expectError     bool
	}{
		{name: "home", dest: "home", expectedContain: "portal.azure.com/#@tenant-123"},
		{name: "empty string", dest: "", expectedContain: "portal.azure.com/#@tenant-123"},
		{name: "subscription", dest: "subscription", expectedContain: "/subscriptions/sub-456/overview"},
		{name: "sub alias", dest: "sub", expectedContain: "/subscriptions/sub-456/overview"},
		{name: "resourcegroups", dest: "resourcegroups", expectedContain: "BrowseResourceGroups"},
		{name: "rg alias", dest: "rg", expectedContain: "BrowseResourceGroups"},
		{name: "vm", dest: "vm", expectedContain: "Microsoft.Compute%2FVirtualMachines"},
		{name: "virtualmachines", dest: "virtualmachines", expectedContain: "Microsoft.Compute%2FVirtualMachines"},
		{name: "storage", dest: "storage", expectedContain: "Microsoft.Storage%2FStorageAccounts"},
		{name: "storageaccounts", dest: "storageaccounts", expectedContain: "Microsoft.Storage%2FStorageAccounts"},
		{name: "network", dest: "network", expectedContain: "Microsoft.Network%2FVirtualNetworks"},
		{name: "vnet", dest: "vnet", expectedContain: "Microsoft.Network%2FVirtualNetworks"},
		{name: "virtualnetworks", dest: "virtualnetworks", expectedContain: "Microsoft.Network%2FVirtualNetworks"},
		{name: "cosmosdb", dest: "cosmosdb", expectedContain: "Microsoft.DocumentDb%2FDatabaseAccounts"},
		{name: "cosmos alias", dest: "cosmos", expectedContain: "Microsoft.DocumentDb%2FDatabaseAccounts"},
		{name: "sql", dest: "sql", expectedContain: "Microsoft.Sql%2FServers"},
		{name: "sqldatabases", dest: "sqldatabases", expectedContain: "Microsoft.Sql%2FServers"},
		{name: "keyvault", dest: "keyvault", expectedContain: "Microsoft.KeyVault%2FVaults"},
		{name: "kv alias", dest: "kv", expectedContain: "Microsoft.KeyVault%2FVaults"},
		{name: "monitor", dest: "monitor", expectedContain: "AzureMonitoringBrowseBlade"},
		{name: "monitoring", dest: "monitoring", expectedContain: "AzureMonitoringBrowseBlade"},
		{name: "aks", dest: "aks", expectedContain: "Microsoft.ContainerService%2FManagedClusters"},
		{name: "kubernetes", dest: "kubernetes", expectedContain: "Microsoft.ContainerService%2FManagedClusters"},
		{name: "functions", dest: "functions", expectedContain: "Microsoft.Web%2FSites/kind/functionapp"},
		{name: "functionapps", dest: "functionapps", expectedContain: "Microsoft.Web%2FSites/kind/functionapp"},
		{name: "appservice", dest: "appservice", expectedContain: "Microsoft.Web%2FSites"},
		{name: "webapps", dest: "webapps", expectedContain: "Microsoft.Web%2FSites"},
		{name: "containers", dest: "containers", expectedContain: "Microsoft.ContainerInstance%2FContainerGroups"},
		{name: "containerinstances", dest: "containerinstances", expectedContain: "Microsoft.ContainerInstance%2FContainerGroups"},
		{
			name:            "full URL passthrough",
			dest:            "https://portal.azure.com/#custom/path",
			expectedContain: "https://portal.azure.com/#custom/path",
			expectError:     false,
		},
		{
			name:        "unsupported destination",
			dest:        "invalid-destination",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveDestination(tt.dest, creds)

			if tt.expectError {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
				return
			}

			require.NoError(t, err)
			assert.Contains(t, result, tt.expectedContain)
			// All URLs should include the tenant context.
			if tt.dest != "" && result != tt.dest {
				assert.Contains(t, result, "@tenant-123")
			}
		})
	}
}

func TestResolveDestination_SubscriptionWithoutID(t *testing.T) {
	// Credentials without subscription ID.
	creds := &types.AzureCredentials{
		TenantID: "tenant-123",
	}

	result, err := ResolveDestination("subscription", creds)
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
	assert.Contains(t, err.Error(), "subscription_id required")
}

func TestResolveDestination_NilCredentials(t *testing.T) {
	// Test that ResolveDestination fails fast with nil credentials.
	result, err := ResolveDestination("home", nil)
	assert.Error(t, err, "ResolveDestination should fail with nil credentials")
	assert.Empty(t, result)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig), "Should return ErrInvalidAuthConfig")
	assert.Contains(t, err.Error(), "Azure credentials are required", "Error message should mention credentials")
}

func TestResolveDestination_EmptyTenantID(t *testing.T) {
	// Test that ResolveDestination fails fast when tenant ID is empty.
	creds := &types.AzureCredentials{
		// TenantID is intentionally empty.
		SubscriptionID: "sub-456",
	}

	result, err := ResolveDestination("home", creds)
	assert.Error(t, err, "ResolveDestination should fail with empty tenant ID")
	assert.Empty(t, result)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig), "Should return ErrInvalidAuthConfig")
	assert.Contains(t, err.Error(), "tenant ID required", "Error message should mention tenant ID")
}

func TestResolveDestination_NilCredentialsWithAlias(t *testing.T) {
	// Test that ResolveDestination fails for any destination when credentials are nil.
	destinations := []string{"", "home", "subscription", "resourcegroups", "vm", "storage"}

	for _, dest := range destinations {
		t.Run(dest, func(t *testing.T) {
			result, err := ResolveDestination(dest, nil)
			assert.Error(t, err, "Should fail with nil credentials for destination: %s", dest)
			assert.Empty(t, result)
			assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
		})
	}
}

func TestResolveDestination_EmptyTenantIDWithAlias(t *testing.T) {
	// Test that ResolveDestination fails for any destination when tenant ID is empty.
	creds := &types.AzureCredentials{
		SubscriptionID: "sub-456",
		// TenantID is intentionally empty.
	}

	destinations := []string{"", "home", "resourcegroups", "vm", "storage"}

	for _, dest := range destinations {
		t.Run(dest, func(t *testing.T) {
			result, err := ResolveDestination(dest, creds)
			assert.Error(t, err, "Should fail with empty tenant ID for destination: %s", dest)
			assert.Empty(t, result)
			assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
		})
	}
}
