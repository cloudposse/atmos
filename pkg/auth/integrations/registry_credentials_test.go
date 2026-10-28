package integrations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExchangeRegistryCredentialsUsesExplicitProviderResult(t *testing.T) {
	setupRegistryTest(t)
	controller := gomock.NewController(t)
	integration := NewMockIntegration(controller)
	exchange := NewMockRegistryCredentialProvider(controller)
	provider := &struct {
		*MockIntegration
		*MockRegistryCredentialProvider
	}{integration, exchange}
	configuration := &IntegrationConfig{Name: "company-registry", Realm: "company", Config: &schema.Integration{Kind: "test/registry"}}
	Register("test/registry", func(actual *IntegrationConfig) (Integration, error) {
		require.Same(t, configuration, actual)
		return provider, nil
	})
	input := &types.AWSCredentials{AccessKeyID: "source-identity"}
	output := &types.OCICredentials{Registry: "registry.example.com", Username: "selected-identity", Password: "selected-token"}
	exchange.EXPECT().ExchangeCredentials(gomock.Any(), input).Return(output, nil)
	actual, err := ExchangeRegistryCredentials(t.Context(), configuration, input)
	require.NoError(t, err)
	require.Same(t, output, actual, "the returned credential must be the selected exchange's result")
	exchange.EXPECT().ExchangeCredentials(gomock.Any(), input).Return(nil, errUtils.ErrAuthenticationFailed)
	_, err = ExchangeRegistryCredentials(t.Context(), configuration, input)
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

func TestExchangeRegistryCredentialsRejectsUnsupportedIntegration(t *testing.T) {
	setupRegistryTest(t)
	_, err := ExchangeRegistryCredentials(t.Context(), nil, nil)
	require.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
	configuration := &IntegrationConfig{Name: "unsupported", Config: &schema.Integration{Kind: "test/no-exchange"}}
	integration := NewMockIntegration(gomock.NewController(t))
	Register("test/no-exchange", func(*IntegrationConfig) (Integration, error) { return integration, nil })
	_, err = ExchangeRegistryCredentials(t.Context(), configuration, nil)
	require.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	require.ErrorContains(t, err, "unsupported")
}
