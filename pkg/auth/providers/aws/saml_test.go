package aws

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/versent/saml2aws/v2/pkg/creds"

	"github.com/versent/saml2aws/v2"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewSAMLProvider_ValidateInputs(t *testing.T) {
	// Wrong kind.
	_, err := NewSAMLProvider("p", &schema.Provider{Kind: "aws/iam-identity-center", URL: "https://idp", Region: "us-east-1"})
	assert.Error(t, err)

	// Missing URL.
	_, err = NewSAMLProvider("p", &schema.Provider{Kind: "aws/saml", Region: "us-east-1"})
	assert.Error(t, err)

	// Missing region.
	_, err = NewSAMLProvider("p", &schema.Provider{Kind: "aws/saml", URL: "https://idp"})
	assert.Error(t, err)

	// Valid.
	p, err := NewSAMLProvider("p", &schema.Provider{Kind: "aws/saml", URL: "https://idp.example.com/saml", Region: "us-east-1"})
	require.NoError(t, err)
	assert.Equal(t, "aws/saml", p.Kind())
	assert.Equal(t, "p", p.Name())
}

func TestSAMLProvider_RequestedSessionSeconds(t *testing.T) {
	p := &samlProvider{config: &schema.Provider{Session: &schema.SessionConfig{Duration: ""}}}
	assert.Equal(t, int32(3600), p.requestedSessionSeconds())

	p.config.Session.Duration = "5m" // less than min -> clamp to 900.
	assert.Equal(t, int32(900), p.requestedSessionSeconds())

	p.config.Session.Duration = "30m"
	assert.Equal(t, int32(1800), p.requestedSessionSeconds())

	p.config.Session.Duration = "13h" // more than max -> clamp to 43200.
	assert.Equal(t, int32(43200), p.requestedSessionSeconds())
}

func TestSAMLProvider_GetProviderType(t *testing.T) {
	p := &samlProvider{config: &schema.Provider{ProviderType: "Okta"}, url: "https://idp"}
	assert.Equal(t, "Okta", p.getProviderType())

	p = &samlProvider{config: &schema.Provider{}, url: "https://accounts.google.com/saml"}
	assert.Equal(t, "Browser", p.getProviderType())

	p = &samlProvider{config: &schema.Provider{}, url: "https://example.okta.com"}
	assert.Equal(t, "Okta", p.getProviderType())

	p = &samlProvider{config: &schema.Provider{}, url: "https://corp/adfs/ls"}
	assert.Equal(t, "ADFS", p.getProviderType())

	p = &samlProvider{config: &schema.Provider{}, url: "https://idp"}
	assert.Equal(t, "Browser", p.getProviderType())
}

func TestSAMLProvider_ValidateAndEnvironment(t *testing.T) {
	p, err := NewSAMLProvider("p", &schema.Provider{Kind: "aws/saml", URL: "://bad url", Region: "us-east-1"})
	require.NoError(t, err)
	assert.Error(t, p.Validate())

	good, err := NewSAMLProvider("p", &schema.Provider{Kind: "aws/saml", URL: "https://idp.example.com/saml", Region: "eu-central-1", DownloadBrowserDriver: true})
	require.NoError(t, err)
	assert.NoError(t, good.Validate())

	env, err := good.Environment()
	require.NoError(t, err)
	assert.Equal(t, "eu-central-1", env["AWS_DEFAULT_REGION"])
	assert.Equal(t, "eu-central-1", env["AWS_REGION"])
	assert.Equal(t, "true", env["SAML2AWS_AUTO_BROWSER_DOWNLOAD"]) // set when download flag is true
}

// stub manager for PreAuthenticate.
type stubSamlMgr struct {
	chain []string
	idmap map[string]schema.Identity
}

func (s stubSamlMgr) Authenticate(context.Context, string) (*types.WhoamiInfo, error) {
	return nil, nil
}
func (s stubSamlMgr) Whoami(context.Context, string) (*types.WhoamiInfo, error) { return nil, nil }
func (s stubSamlMgr) Validate() error                                           { return nil }
func (s stubSamlMgr) GetDefaultIdentity() (string, error)                       { return "", nil }
func (s stubSamlMgr) ListIdentities() []string                                  { return nil }
func (s stubSamlMgr) GetProviderForIdentity(string) string                      { return "" }
func (s stubSamlMgr) GetProviderKindForIdentity(string) (string, error)         { return "", nil }
func (s stubSamlMgr) GetChain() []string                                        { return s.chain }
func (s stubSamlMgr) GetStackInfo() *schema.ConfigAndStacksInfo                 { return nil }
func (s stubSamlMgr) ListProviders() []string                                   { return nil }
func (s stubSamlMgr) GetIdentities() map[string]schema.Identity                 { return s.idmap }
func (s stubSamlMgr) GetProviders() map[string]schema.Provider                  { return nil }

func TestSAMLProvider_PreAuthenticate(t *testing.T) {
	p, err := NewSAMLProvider("p", &schema.Provider{Kind: "aws/saml", URL: "https://idp.example.com/saml", Region: "us-east-1"})
	require.NoError(t, err)
	sp := p.(*samlProvider)

	// Chain too short -> no change, no error.
	err = sp.PreAuthenticate(stubSamlMgr{chain: []string{"prov"}, idmap: map[string]schema.Identity{}})
	assert.NoError(t, err)

	// Missing identity referenced -> error.
	err = sp.PreAuthenticate(stubSamlMgr{chain: []string{"prov", "dev"}, idmap: map[string]schema.Identity{}})
	assert.Error(t, err)

	// Identity exists but missing assume_role -> error.
	err = sp.PreAuthenticate(stubSamlMgr{chain: []string{"prov", "dev"}, idmap: map[string]schema.Identity{
		"dev": {Kind: "aws/assume-role", Principal: map[string]any{}},
	}})
	assert.Error(t, err)

	// Proper identity -> captures hint.
	err = sp.PreAuthenticate(stubSamlMgr{chain: []string{"prov", "dev"}, idmap: map[string]schema.Identity{
		"dev": {Kind: "aws/assume-role", Principal: map[string]any{"assume_role": "arn:aws:iam::123:role/Dev"}},
	}})
	require.NoError(t, err)
	assert.Contains(t, sp.RoleToAssumeFromAssertion, "arn:aws:iam::123:role/Dev")
}

func TestSAMLProvider_selectRole(t *testing.T) {
	sp := &samlProvider{RoleToAssumeFromAssertion: "dev"}
	roles := []*saml2aws.AWSRole{
		{RoleARN: "arn:aws:iam::123:role/Prod", PrincipalARN: "arn:aws:iam::123:saml-provider/idp"},
		{RoleARN: "arn:aws:iam::123:role/DevAccess", PrincipalARN: "arn:aws:iam::123:saml-provider/idp"},
	}

	sel := sp.selectRole(roles)
	require.NotNil(t, sel)
	assert.Equal(t, "arn:aws:iam::123:role/DevAccess", sel.RoleARN)

	// No hint match -> first.
	sp.RoleToAssumeFromAssertion = "nonexistent"
	sel = sp.selectRole(roles)
	require.NotNil(t, sel)
	assert.Equal(t, "arn:aws:iam::123:role/Prod", sel.RoleARN)
}

func TestSAMLProvider_setupBrowserAutomation_SetsEnv(t *testing.T) {
    t.Setenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD", "")
    pAny, err := NewSAMLProvider("p", &schema.Provider{Kind: "aws/saml", URL: "https://idp.example.com/saml", Region: "us-east-1", DownloadBrowserDriver: true})
    require.NoError(t, err)
    sp := pAny.(*samlProvider)
    sp.setupBrowserAutomation()
    // The function should set this env var when DownloadBrowserDriver is true.
    assert.Equal(t, "true", os.Getenv("SAML2AWS_AUTO_BROWSER_DOWNLOAD"))
}

func TestSAMLProvider_Authenticate_RequiresRoleHint(t *testing.T) {
    // Ensure it fails early without RoleToAssumeFromAssertion and does not perform network calls.
    pAny, err := NewSAMLProvider("p", &schema.Provider{Kind: "aws/saml", URL: "https://idp.example.com/saml", Region: "us-east-1"})
    require.NoError(t, err)
    sp := pAny.(*samlProvider)
    _, err = sp.Authenticate(context.Background())
    assert.Error(t, err)
}


type stubSAMLClient struct{
	assertion string
	err error
}

// Ensure our stub implements SAMLClient to avoid unused import and verify signature.
var _ saml2aws.SAMLClient = (*stubSAMLClient)(nil)

func (s stubSAMLClient) Authenticate(_ *creds.LoginDetails) (string, error) { return s.assertion, s.err }
func (s stubSAMLClient) Validate(_ *creds.LoginDetails) error { return nil }

func TestSAMLProvider_createSAMLConfig_LoginDetails(t *testing.T) {
	p, err := NewSAMLProvider("p", &schema.Provider{Kind: "aws/saml", URL: "https://idp.example.com", Region: "eu-west-1", Username: "user", Password: "pass", DownloadBrowserDriver: true})
	require.NoError(t, err)
	sp := p.(*samlProvider)

	cfg := sp.createSAMLConfig()
	assert.Equal(t, "https://idp.example.com", cfg.URL)
	assert.Equal(t, "p", cfg.Profile)
	assert.Equal(t, "eu-west-1", cfg.Region)
	assert.True(t, cfg.DownloadBrowser)

	ld := sp.createLoginDetails()
	assert.Equal(t, "https://idp.example.com", ld.URL)
	assert.Equal(t, "user", ld.Username)
	assert.Equal(t, "pass", ld.Password)
}

func TestSAMLProvider_authenticateAndGetAssertion_SuccessAndEmpty(t *testing.T) {
	sp := &samlProvider{name: "p", url: "https://idp", region: "us-east-1"}

    // Success.
	out, err := sp.authenticateAndGetAssertion(stubSAMLClient{assertion: "abc"}, &creds.LoginDetails{})
	require.NoError(t, err)
	assert.Equal(t, "abc", out)

    // Empty -> error.
	out, err = sp.authenticateAndGetAssertion(stubSAMLClient{assertion: ""}, &creds.LoginDetails{})
	assert.Error(t, err)
	assert.Equal(t, "", out)
}
