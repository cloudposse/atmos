package aws

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRolesFromAssertion(t *testing.T) {
	assertion, err := os.ReadFile(filepath.Join("saml2aws", "testdata", "assertion.xml"))
	require.NoError(t, err)

	roles, err := rolesFromAssertion(base64.StdEncoding.EncodeToString(assertion))
	require.NoError(t, err)
	require.NotEmpty(t, roles)
	for _, role := range roles {
		assert.Contains(t, role.RoleARN, ":role/")
		assert.Contains(t, role.PrincipalARN, ":saml-provider/")
	}
}

func TestRolesFromAssertionErrors(t *testing.T) {
	tests := []struct {
		name      string
		assertion string
	}{
		{name: "not base64", assertion: "%%%not-base64%%%"},
		{name: "not XML", assertion: base64.StdEncoding.EncodeToString([]byte("<<<"))},
		{name: "no role attribute", assertion: base64.StdEncoding.EncodeToString([]byte(`<Response xmlns="urn:oasis:names:tc:SAML:2.0:protocol"></Response>`))},
		{
			name: "malformed role value",
			assertion: base64.StdEncoding.EncodeToString([]byte(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">` +
				`<saml:Assertion><saml:AttributeStatement><saml:Attribute Name="https://aws.amazon.com/SAML/Attributes/Role">` +
				`<saml:AttributeValue>not-a-role-pair</saml:AttributeValue></saml:Attribute></saml:AttributeStatement></saml:Assertion></samlp:Response>`)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := rolesFromAssertion(tt.assertion)
			require.ErrorIs(t, err, errUtils.ErrAwsSAMLDecodeFailed)
		})
	}
}

// Non-Browser drivers use API/HTTP flows and must not touch Playwright or browser storage.
func TestPrepareBrowserDriverSkipsNonBrowserDrivers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	p := &samlProvider{config: &schema.Provider{}}
	for _, driver := range []string{"GoogleApps", "Okta", "ADFS"} {
		require.NoError(t, p.prepareBrowserDriver(driver))
	}

	// No browser storage directory was created.
	_, err := os.Stat(filepath.Join(home, ".aws", "saml2aws"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}
