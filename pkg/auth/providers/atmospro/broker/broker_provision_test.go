package broker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProBroker_Provision_NoIdentity(t *testing.T) {
	// No atmos/pro github/sts identity configured → Provision is a no-op returning (nil, nil).
	env, err := proBroker{}.Provision(context.Background(), &schema.AtmosConfiguration{})
	require.NoError(t, err)
	assert.Nil(t, env)
}

func TestProBroker_Provision_AuthFails(t *testing.T) {
	// An identity resolves, so Provision builds a manager and calls EnsureIdentityEnvironment.
	// With no workspace ID, the atmos/pro provider fails fast (before any network) and the error
	// propagates out of Provision.
	t.Setenv("ATMOS_PRO_WORKSPACE_ID", "")
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())

	atmosConfig := &schema.AtmosConfiguration{
		Auth: proAuthConfig(schema.Integration{Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}}),
	}

	_, err := proBroker{}.Provision(context.Background(), atmosConfig)
	require.Error(t, err)
}
