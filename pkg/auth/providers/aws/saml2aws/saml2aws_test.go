package saml2aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

func TestProviderList_Keys(t *testing.T) {
	names := MFAsByProvider.Names()

	require.Equal(t, []string{"ADFS", "Browser", "GoogleApps", "Okta"}, names)
}

func TestProviderList_Mfas(t *testing.T) {
	mfas := MFAsByProvider.Mfas("ADFS")

	require.Equal(t, []string{"Auto", "Azure", "Defender", "VIP"}, mfas)
}

func TestProviderList_MfasDoesNotMutate(t *testing.T) {
	before := append([]string(nil), MFAsByProvider["Okta"]...)

	_ = MFAsByProvider.Mfas("Okta")

	require.Equal(t, before, MFAsByProvider["Okta"])
}

func TestProviderInvalid(t *testing.T) {
	account := &cfg.IDPAccount{
		Provider: "foo1",
	}
	_, err := NewSAMLClient(account)
	require.ErrorIs(t, err, ErrUnsupportedProvider)
	assert.ErrorContains(t, err, `"foo1"`)
	assert.ErrorContains(t, err, "ADFS, Browser, GoogleApps, Okta")
}

// Drivers removed in the Atmos fork must fail with a clear, typed error.
func TestProviderRemovedDrivers(t *testing.T) {
	for _, provider := range []string{"AzureAD", "Ping", "KeyCloak", "OneLogin", "Shell", "Auth0"} {
		t.Run(provider, func(t *testing.T) {
			_, err := NewSAMLClient(&cfg.IDPAccount{Provider: provider, MFA: "Auto"})
			require.ErrorIs(t, err, ErrUnsupportedProvider)
		})
	}
}

func TestProviderInvalidMFA(t *testing.T) {
	for _, provider := range []string{"ADFS", "Okta", "GoogleApps"} {
		t.Run(provider, func(t *testing.T) {
			_, err := NewSAMLClient(&cfg.IDPAccount{Provider: provider, MFA: "bogus"})
			require.ErrorIs(t, err, ErrInvalidMFA)
		})
	}
}

func TestProviderValidClients(t *testing.T) {
	loginDetails := &creds.LoginDetails{Username: "testuser", Password: "testtestlol", URL: "https://id.example.com", MFAToken: "123456"}

	for _, provider := range []string{"ADFS", "Okta", "GoogleApps", "Browser"} {
		t.Run(provider, func(t *testing.T) {
			client, err := NewSAMLClient(&cfg.IDPAccount{Provider: provider, MFA: "Auto", URL: "https://id.example.com"})
			require.NoError(t, err)
			require.NotNil(t, client)
			require.NoError(t, client.Validate(loginDetails))
		})
	}
}

// The Browser driver ignores MFA, matching upstream.
func TestProviderBrowserIgnoresMFA(t *testing.T) {
	client, err := NewSAMLClient(&cfg.IDPAccount{Provider: "Browser", MFA: "anything"})
	require.NoError(t, err)
	require.NotNil(t, client)
}
