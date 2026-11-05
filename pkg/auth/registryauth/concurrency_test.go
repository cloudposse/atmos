package registryauth

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConcurrentRegistryResolutionSharesExchangeForLiveSession(t *testing.T) {
	const workers = 16
	const host = "123456789012.dkr.ecr.us-east-1.amazonaws.com"
	resolver, manager, _ := testResolver(t)
	repository, err := name.NewRepository(host + "/plugins/fixture")
	require.NoError(t, err)
	resolver.bindings = map[string]string{host: "corporate"}
	resolver.config.Auth.Identities["corporate"] = schema.Identity{Kind: "aws/user"}
	resolver.config.Auth.Integrations = map[string]schema.Integration{"ecr": {
		Kind: "aws/ecr", Via: &schema.IntegrationVia{Identity: "corporate"},
		Spec: &schema.IntegrationSpec{Registry: &schema.Registry{AccountID: "123456789012", Region: "us-east-1"}},
	}}
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(&types.WhoamiInfo{
		Credentials: &types.AWSCredentials{AccessKeyID: "selected-session", SecretAccessKey: "secret"},
	}, nil).Times(workers)
	var exchanges atomic.Int32
	resolver.exchange = func(context.Context, *integrations.IntegrationConfig, types.ICredentials) (*types.OCICredentials, error) {
		exchanges.Add(1)
		return &types.OCICredentials{Registry: host, Username: "AWS", Password: "selected-token"}, nil
	}
	start := make(chan struct{})
	var consumers sync.WaitGroup
	for range workers {
		consumers.Go(func() {
			<-start
			authenticator, matched, resolveErr := resolver.Resolve(t.Context(), repository)
			if resolveErr != nil || !matched || authenticator == nil {
				t.Errorf("bound resolution failed: matched=%t, err=%v", matched, resolveErr)
				return
			}
			credentials, authErr := authenticator.Authorization()
			if authErr != nil || credentials == nil || credentials.Password != "selected-token" {
				t.Errorf("resolution did not return the selected session token: %v", authErr)
			}
		})
	}
	close(start)
	consumers.Wait()
	require.EqualValues(t, 1, exchanges.Load(), "concurrent requests must share the exchange while checking the live identity session")
}
