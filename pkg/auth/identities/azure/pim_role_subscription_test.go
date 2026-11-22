package azure

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

const pimScopeSubscription = "11111111-2222-3333-4444-555555555555"

func sandboxPIMHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	homedir.Reset()
	homedir.DisableCache = true
	t.Cleanup(func() {
		homedir.Reset()
		homedir.DisableCache = false
	})
	return tmpHome
}

// TestPIMRole_PostAuthenticate_UsesScopeSubscription verifies that the subscription
// exported to Terraform (auth context and ARM_SUBSCRIPTION_ID) is the one from
// principal.scope, the same subscription Environment() and the az CLI files use,
// and not the parent identity's subscription.
func TestPIMRole_PostAuthenticate_UsesScopeSubscription(t *testing.T) {
	tests := []struct {
		name            string
		parentSubscript string
	}{
		{name: "parent has a different subscription", parentSubscript: "parent-subscription"},
		{name: "parent has no subscription", parentSubscript: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpHome := sandboxPIMHome(t)

			principal := defaultPrincipal()
			principal["scope"] = "/subscriptions/" + pimScopeSubscription + "/resourceGroups/rg1"
			id := newTestIdentity(t, principal, &mockPIMClient{})
			id.SetRealm("test-realm")

			parent := testAzureCreds()
			parent.SubscriptionID = tt.parentSubscript
			authContext := &schema.AuthContext{}
			stackInfo := &schema.ConfigAndStacksInfo{}
			require.NoError(t, id.PostAuthenticate(context.Background(), &authTypes.PostAuthenticateParams{
				AuthContext:  authContext,
				StackInfo:    stackInfo,
				ProviderName: "azure-interactive",
				IdentityName: "prod-contributor",
				Credentials:  parent,
				Realm:        "test-realm",
			}))

			require.NotNil(t, authContext.Azure)
			assert.Equal(t, pimScopeSubscription, authContext.Azure.SubscriptionID)
			assert.Equal(t, pimScopeSubscription, stackInfo.ComponentEnvSection["ARM_SUBSCRIPTION_ID"])
			assert.Equal(t, pimScopeSubscription, stackInfo.ComponentEnvSection["AZURE_SUBSCRIPTION_ID"])

			// The parent credentials are shared with the parent identity and must not be mutated.
			assert.Equal(t, tt.parentSubscript, parent.SubscriptionID)

			// The stored credentials agree with the exported subscription.
			data, err := os.ReadFile(filepath.Join(tmpHome, ".azure", "atmos", "test-realm", "azure-interactive", "credentials.json"))
			require.NoError(t, err)
			var stored authTypes.AzureCredentials
			require.NoError(t, json.Unmarshal(data, &stored))
			assert.Equal(t, pimScopeSubscription, stored.SubscriptionID)
		})
	}
}
