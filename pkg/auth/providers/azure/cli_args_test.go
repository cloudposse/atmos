package azure

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
)

// TestCLIProvider_ExecuteAzCommand_TenantSubscriptionArgs verifies that az is
// never invoked with both --tenant and --subscription, which az rejects with
// "Please specify only one of subscription and tenant, not both".
func TestCLIProvider_ExecuteAzCommand_TenantSubscriptionArgs(t *testing.T) {
	tests := []struct {
		name           string
		subscriptionID string
		resource       string
		wantArgs       []string
	}{
		{
			name:           "subscription set passes only --subscription",
			subscriptionID: "sub-456",
			wantArgs:       []string{"account", "get-access-token", "--subscription", "sub-456", "--output", "json"},
		},
		{
			name:     "no subscription passes --tenant",
			wantArgs: []string{"account", "get-access-token", "--tenant", "tenant-123", "--output", "json"},
		},
		{
			name:           "subscription with resource",
			subscriptionID: "sub-456",
			resource:       "custom-server-id",
			wantArgs:       []string{"account", "get-access-token", "--subscription", "sub-456", "--resource", "custom-server-id", "--output", "json"},
		},
		{
			name:     "tenant with resource",
			resource: "custom-server-id",
			wantArgs: []string{"account", "get-access-token", "--tenant", "tenant-123", "--resource", "custom-server-id", "--output", "json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotName string
			var gotArgs []string
			originalNewCommandContext := newCommandContext
			t.Cleanup(func() { newCommandContext = originalNewCommandContext })
			newCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				gotName = name
				gotArgs = append([]string(nil), args...)
				cmdArgs := append([]string{"-test.run=^TestCLIProviderHelperProcess$", "--"}, args...)
				cmd := exec.CommandContext(ctx, os.Args[0], cmdArgs...)
				cmd.Env = append(os.Environ(), azureCLIHelperEnv+"=1")
				return cmd
			}

			provider := &cliProvider{
				name:           "test",
				tenantID:       "tenant-123",
				subscriptionID: tt.subscriptionID,
				cloudEnv:       azureCloud.GetCloudEnvironment(""),
			}
			_, err := provider.executeAzCommand(context.Background(), tt.resource)
			require.NoError(t, err)
			assert.Equal(t, "az", gotName)
			assert.Equal(t, tt.wantArgs, gotArgs)
		})
	}
}
