package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	stsTypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/versent/saml2aws/v2"
	"github.com/versent/saml2aws/v2/pkg/creds"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
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
	// Explicit driver config always wins, regardless of Playwright driver availability.
	p := &samlProvider{config: &schema.Provider{ProviderType: "Okta"}, url: "https://idp"}
	assert.Equal(t, "Okta", p.getDriver())

	p = &samlProvider{config: &schema.Provider{Driver: "GoogleApps"}, url: "https://idp"}
	assert.Equal(t, "GoogleApps", p.getDriver())

	// Without explicit driver config, behavior depends on Playwright driver availability.
	// If drivers are available or can be downloaded, Browser is preferred.
	// Otherwise, falls back to provider-specific types based on URL.
	t.Run("provider-specific-fallback-or-browser", func(t *testing.T) {
		testCases := []struct {
			name             string
			url              string
			expectedFallback string // Expected when no Playwright drivers
		}{
			{
				name:             "google",
				url:              "https://accounts.google.com/saml",
				expectedFallback: "GoogleApps",
			},
			{
				name:             "okta",
				url:              "https://example.okta.com",
				expectedFallback: "Okta",
			},
			{
				name:             "adfs",
				url:              "https://corp/adfs/ls",
				expectedFallback: "ADFS",
			},
			{
				name:             "unknown",
				url:              "https://idp",
				expectedFallback: "Browser", // Unknown provider defaults to Browser
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Disable auto-download to avoid affecting fallback behavior.
				p := &samlProvider{config: &schema.Provider{DownloadBrowserDriver: false}, url: tc.url}
				driver := p.getDriver()

				// If Playwright drivers are installed, Browser is returned.
				// Otherwise, provider-specific type is returned.
				if p.playwrightDriversInstalled() {
					assert.Equal(t, "Browser", driver, "With Playwright drivers installed, should use Browser")
				} else {
					assert.Equal(t, tc.expectedFallback, driver, "Without Playwright drivers, should fall back to provider-specific type")
				}
			})
		}
	})

	// When DownloadBrowserDriver is explicitly enabled, always expect Browser.
	t.Run("download-enabled", func(t *testing.T) {
		p := &samlProvider{config: &schema.Provider{DownloadBrowserDriver: true}, url: "https://accounts.google.com/saml"}
		assert.Equal(t, "Browser", p.getDriver(), "With download enabled, should always use Browser")
	})
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

func (s stubSamlMgr) GetCachedCredentials(context.Context, string) (*types.WhoamiInfo, error) {
	return nil, nil
}

func (s stubSamlMgr) Authenticate(context.Context, string) (*types.WhoamiInfo, error) {
	return nil, nil
}

func (s stubSamlMgr) AuthenticateProvider(context.Context, string) (*types.WhoamiInfo, error) {
	return nil, nil
}
func (s stubSamlMgr) Whoami(context.Context, string) (*types.WhoamiInfo, error) { return nil, nil }
func (s stubSamlMgr) Validate() error                                           { return nil }
func (s stubSamlMgr) GetDefaultIdentity(_ bool) (string, error)                 { return "", nil }
func (s stubSamlMgr) ListIdentities() []string                                  { return nil }
func (s stubSamlMgr) GetProviderForIdentity(string) string                      { return "" }
func (s stubSamlMgr) GetFilesDisplayPath(string) string                         { return "~/.aws/atmos" }
func (s stubSamlMgr) GetProviderKindForIdentity(string) (string, error)         { return "", nil }
func (s stubSamlMgr) GetChain() []string                                        { return s.chain }
func (s stubSamlMgr) GetStackInfo() *schema.ConfigAndStacksInfo                 { return nil }
func (s stubSamlMgr) ListProviders() []string                                   { return nil }
func (s stubSamlMgr) GetIdentities() map[string]schema.Identity                 { return s.idmap }
func (s stubSamlMgr) GetProviders() map[string]schema.Provider                  { return nil }
func (s stubSamlMgr) Logout(context.Context, string, bool) error                { return nil }
func (s stubSamlMgr) LogoutProvider(context.Context, string, bool) error        { return nil }
func (s stubSamlMgr) LogoutAll(context.Context, bool) error                     { return nil }
func (s stubSamlMgr) GetEnvironmentVariables(string) (map[string]string, error) {
	return make(map[string]string), nil
}

func (s stubSamlMgr) PrepareShellEnvironment(context.Context, string, []string) ([]string, error) {
	return nil, nil
}

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

type stubSAMLClient struct {
	assertion string
	err       error
}

// Ensure our stub implements SAMLClient to avoid unused import and verify signature.
var _ saml2aws.SAMLClient = (*stubSAMLClient)(nil)

func (s stubSAMLClient) Authenticate(_ *creds.LoginDetails) (string, error) {
	return s.assertion, s.err
}
func (s stubSAMLClient) Validate(_ *creds.LoginDetails) error { return nil }

type stubSTSClient struct {
	output        *sts.AssumeRoleWithSAMLOutput
	err           error
	capturedInput *sts.AssumeRoleWithSAMLInput
}

func (s *stubSTSClient) AssumeRoleWithSAML(_ context.Context, params *sts.AssumeRoleWithSAMLInput, _ ...func(*sts.Options)) (*sts.AssumeRoleWithSAMLOutput, error) {
	s.capturedInput = params
	if s.err != nil {
		return nil, s.err
	}
	return s.output, nil
}

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

func Test_samlProvider_assumeRoleWithSAML(t *testing.T) {
	type fields struct {
		name                      string
		config                    *schema.Provider
		url                       string
		region                    string
		RoleToAssumeFromAssertion string
	}
	type args struct {
		ctx           context.Context
		samlAssertion string
		role          *saml2aws.AWSRole
	}
	type (
		configLoader  func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error)
		clientFactory func(aws.Config) assumeRoleWithSAMLClient
	)
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *types.AWSCredentials
		wantErr assert.ErrorAssertionFunc
		setup   func(t *testing.T) (configLoader, clientFactory, *stubSTSClient)
		after   func(t *testing.T, stub *stubSTSClient)
	}{
		{
			name: "successful assume role",
			fields: fields{
				region: "us-east-1",
				config: &schema.Provider{Session: &schema.SessionConfig{Duration: "2h"}},
			},
			args: args{
				ctx:           context.Background(),
				samlAssertion: "ZmFrZS1hc3NlcnRpb24=",
				role: &saml2aws.AWSRole{
					RoleARN:      "arn:aws:iam::123456789012:role/Dev",
					PrincipalARN: "arn:aws:iam::123456789012:saml-provider/idp",
				},
			},
			want: &types.AWSCredentials{
				AccessKeyID:     "ASIAEXAMPLE",
				SecretAccessKey: "secret",
				SessionToken:    "session",
				Region:          "us-east-1",
				Expiration:      "2024-01-02T03:04:05Z",
			},
			wantErr: assert.NoError,
			setup: func(t *testing.T) (configLoader, clientFactory, *stubSTSClient) {
				stub := &stubSTSClient{
					output: &sts.AssumeRoleWithSAMLOutput{
						Credentials: &stsTypes.Credentials{
							AccessKeyId:     aws.String("ASIAEXAMPLE"),
							SecretAccessKey: aws.String("secret"),
							SessionToken:    aws.String("session"),
							Expiration:      aws.Time(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)),
						},
					},
				}
				loader := func(_ context.Context, _ ...func(*config.LoadOptions) error) (aws.Config, error) {
					return aws.Config{Region: "us-east-1"}, nil
				}
				factory := func(cfg aws.Config) assumeRoleWithSAMLClient {
					assert.Equal(t, "us-east-1", cfg.Region)
					return stub
				}
				return loader, factory, stub
			},
			after: func(t *testing.T, stub *stubSTSClient) {
				require.NotNil(t, stub)
				require.NotNil(t, stub.capturedInput)
				assert.Equal(t, "ZmFrZS1hc3NlcnRpb24=", aws.ToString(stub.capturedInput.SAMLAssertion))
				assert.Equal(t, "arn:aws:iam::123456789012:role/Dev", aws.ToString(stub.capturedInput.RoleArn))
				assert.Equal(t, "arn:aws:iam::123456789012:saml-provider/idp", aws.ToString(stub.capturedInput.PrincipalArn))
				assert.Equal(t, int32(7200), aws.ToInt32(stub.capturedInput.DurationSeconds))
			},
		},
		{
			name:   "config load failure",
			fields: fields{region: "us-west-1"},
			args: args{
				ctx:           context.Background(),
				samlAssertion: "anything",
				role: &saml2aws.AWSRole{
					RoleARN:      "arn:aws:iam::123456789012:role/Test",
					PrincipalARN: "arn:aws:iam::123456789012:saml-provider/idp",
				},
			},
			want:    nil,
			wantErr: assert.Error,
			setup: func(t *testing.T) (configLoader, clientFactory, *stubSTSClient) {
				loader := func(_ context.Context, _ ...func(*config.LoadOptions) error) (aws.Config, error) {
					return aws.Config{}, errors.New("config boom")
				}
				factory := func(cfg aws.Config) assumeRoleWithSAMLClient {
					return &stubSTSClient{}
				}
				return loader, factory, nil
			},
		},
		{
			name:   "sts error bubbles up",
			fields: fields{region: "us-west-2"},
			args: args{
				ctx:           context.Background(),
				samlAssertion: "ZmFpbC1hc3NlcnRpb24=",
				role: &saml2aws.AWSRole{
					RoleARN:      "arn:aws:iam::999999999999:role/Fail",
					PrincipalARN: "arn:aws:iam::999999999999:saml-provider/idp",
				},
			},
			want:    nil,
			wantErr: assert.Error,
			setup: func(t *testing.T) (configLoader, clientFactory, *stubSTSClient) {
				stub := &stubSTSClient{err: errors.New("sts failure")}
				loader := func(_ context.Context, _ ...func(*config.LoadOptions) error) (aws.Config, error) {
					return aws.Config{Region: "us-west-2"}, nil
				}
				factory := func(cfg aws.Config) assumeRoleWithSAMLClient {
					assert.Equal(t, "us-west-2", cfg.Region)
					return stub
				}
				return loader, factory, stub
			},
		},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			var (
				loader  configLoader
				factory clientFactory
				stub    *stubSTSClient
			)
			if tc.setup != nil {
				loader, factory, stub = tc.setup(t)
			}
			if loader == nil {
				loader = config.LoadDefaultConfig
			}
			if factory == nil {
				factory = func(cfg aws.Config) assumeRoleWithSAMLClient {
					return sts.NewFromConfig(cfg)
				}
			}
			p := &samlProvider{
				name:                      tc.fields.name,
				config:                    tc.fields.config,
				url:                       tc.fields.url,
				region:                    tc.fields.region,
				RoleToAssumeFromAssertion: tc.fields.RoleToAssumeFromAssertion,
			}
			got, err := p.assumeRoleWithSAMLWithDeps(tc.args.ctx, tc.args.samlAssertion, tc.args.role, loader, factory)
			if !tc.wantErr(t, err, fmt.Sprintf("assumeRoleWithSAML(%v, %v, %v)", tc.args.ctx, tc.args.samlAssertion, tc.args.role)) {
				return
			}
			assert.Equalf(t, tc.want, got, "assumeRoleWithSAML(%v, %v, %v)", tc.args.ctx, tc.args.samlAssertion, tc.args.role)
			if tc.after != nil {
				tc.after(t, stub)
			}
		})
	}
}

func TestSAMLProvider_createSAMLConfig_AllFields(t *testing.T) {
	p, err := NewSAMLProvider("test-provider", &schema.Provider{
		Kind:                  "aws/saml",
		URL:                   "https://idp.example.com/saml",
		Region:                "eu-central-1",
		Username:              "testuser",
		Password:              "testpass",
		ProviderType:          "Okta",
		DownloadBrowserDriver: true,
		Session:               &schema.SessionConfig{Duration: "2h"},
	})
	require.NoError(t, err)
	sp := p.(*samlProvider)

	cfg := sp.createSAMLConfig()
	assert.Equal(t, "https://idp.example.com/saml", cfg.URL)
	assert.Equal(t, "test-provider", cfg.Profile)
	assert.Equal(t, "eu-central-1", cfg.Region)
	assert.Equal(t, "testuser", cfg.Username)
	assert.Equal(t, "Okta", cfg.Provider)
	assert.Equal(t, "Auto", cfg.MFA)
	assert.False(t, cfg.SkipVerify)
	assert.Equal(t, 30, cfg.Timeout)
	assert.Equal(t, "urn:amazon:webservices", cfg.AmazonWebservicesURN)
	assert.True(t, cfg.DownloadBrowser)
	assert.False(t, cfg.Headless)
}

func TestSAMLProvider_createLoginDetails_WithPassword(t *testing.T) {
	p, err := NewSAMLProvider("test-provider", &schema.Provider{
		Kind:     "aws/saml",
		URL:      "https://idp.example.com/saml",
		Region:   "us-east-1",
		Username: "testuser",
		Password: "secretpass",
	})
	require.NoError(t, err)
	sp := p.(*samlProvider)

	ld := sp.createLoginDetails()
	assert.Equal(t, "https://idp.example.com/saml", ld.URL)
	assert.Equal(t, "testuser", ld.Username)
	assert.Equal(t, "secretpass", ld.Password)
}

func TestSAMLProvider_createLoginDetails_NoPassword(t *testing.T) {
	p, err := NewSAMLProvider("test-provider", &schema.Provider{
		Kind:     "aws/saml",
		URL:      "https://idp.example.com/saml",
		Region:   "us-east-1",
		Username: "testuser",
		// No password provided
	})
	require.NoError(t, err)
	sp := p.(*samlProvider)

	ld := sp.createLoginDetails()
	assert.Equal(t, "https://idp.example.com/saml", ld.URL)
	assert.Equal(t, "testuser", ld.Username)
	assert.Empty(t, ld.Password) // Should be empty when not provided
}

func TestSAMLProvider_selectRole_MultipleRoles(t *testing.T) {
	sp := &samlProvider{RoleToAssumeFromAssertion: "DevAccess"}
	roles := []*saml2aws.AWSRole{
		{RoleARN: "arn:aws:iam::123:role/ProdAccess", PrincipalARN: "arn:aws:iam::123:saml-provider/idp"},
		{RoleARN: "arn:aws:iam::123:role/DevAccess", PrincipalARN: "arn:aws:iam::123:saml-provider/idp"},
		{RoleARN: "arn:aws:iam::123:role/TestAccess", PrincipalARN: "arn:aws:iam::123:saml-provider/idp"},
	}

	// Should select the role matching the hint
	sel := sp.selectRole(roles)
	require.NotNil(t, sel)
	assert.Equal(t, "arn:aws:iam::123:role/DevAccess", sel.RoleARN)
}

func TestSAMLProvider_selectRole_NoRoles(t *testing.T) {
	sp := &samlProvider{RoleToAssumeFromAssertion: "AnyRole"}
	roles := []*saml2aws.AWSRole{}

	sel := sp.selectRole(roles)
	assert.Nil(t, sel)
}

func TestSAMLProvider_selectRole_PartialMatch(t *testing.T) {
	sp := &samlProvider{RoleToAssumeFromAssertion: "dev"}
	roles := []*saml2aws.AWSRole{
		{RoleARN: "arn:aws:iam::123:role/Production", PrincipalARN: "arn:aws:iam::123:saml-provider/idp"},
		{RoleARN: "arn:aws:iam::123:role/Development", PrincipalARN: "arn:aws:iam::123:saml-provider/idp"},
	}

	// Should match "Development" because it contains "dev"
	sel := sp.selectRole(roles)
	require.NotNil(t, sel)
	assert.Equal(t, "arn:aws:iam::123:role/Development", sel.RoleARN)
}

func TestSAMLProvider_WithCustomResolver(t *testing.T) {
	// Test SAML provider with custom resolver configuration
	config := &schema.Provider{
		Kind:   "aws/saml",
		Region: "us-east-1",
		URL:    "https://idp.example.com/saml",
		Spec: map[string]interface{}{
			"aws": map[string]interface{}{
				"resolver": map[string]interface{}{
					"url": "http://localhost:4566",
				},
			},
		},
	}

	p, err := NewSAMLProvider("saml-localstack", config)
	require.NoError(t, err)
	assert.NotNil(t, p)

	// Cast to concrete type to access internal fields
	sp, ok := p.(*samlProvider)
	require.True(t, ok)
	assert.Equal(t, "saml-localstack", sp.name)
	assert.Equal(t, "us-east-1", sp.region)
	assert.Equal(t, "https://idp.example.com/saml", sp.url)

	// Verify the provider has the config with resolver
	assert.NotNil(t, sp.config)
	assert.NotNil(t, sp.config.Spec)
	awsSpec, exists := sp.config.Spec["aws"]
	assert.True(t, exists)
	assert.NotNil(t, awsSpec)
}

func TestSAMLProvider_WithoutCustomResolver(t *testing.T) {
	// Test SAML provider without custom resolver configuration
	config := &schema.Provider{
		Kind:   "aws/saml",
		Region: "us-east-1",
		URL:    "https://idp.example.com/saml",
	}

	p, err := NewSAMLProvider("saml-standard", config)
	require.NoError(t, err)
	assert.NotNil(t, p)

	// Cast to concrete type
	sp, ok := p.(*samlProvider)
	require.True(t, ok)
	assert.Equal(t, "saml-standard", sp.name)

	// Verify the provider works without resolver config
	assert.NoError(t, p.Validate())
}

func TestSAMLProvider_shouldDownloadBrowser(t *testing.T) {
	tests := []struct {
		name                string
		config              *schema.Provider
		driverValue         string
		playwrightInstalled bool
		expectedResult      bool
	}{
		{
			name:                "explicit download flag set to true",
			config:              &schema.Provider{DownloadBrowserDriver: true},
			driverValue:         "Browser",
			playwrightInstalled: false,
			expectedResult:      true,
		},
		{
			name:                "driver is not Browser",
			config:              &schema.Provider{},
			driverValue:         "Okta",
			playwrightInstalled: false,
			expectedResult:      false,
		},
		{
			name:                "playwright drivers already installed",
			config:              &schema.Provider{},
			driverValue:         "Browser",
			playwrightInstalled: true,
			expectedResult:      false,
		},
		{
			name:                "no drivers installed, enables auto-download",
			config:              &schema.Provider{},
			driverValue:         "Browser",
			playwrightInstalled: false,
			expectedResult:      true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			sp := &samlProvider{
				config: tc.config,
			}

			// Mock driver value.
			sp.config.Driver = tc.driverValue

			// Mock playwrightDriversInstalled based on test case.
			if tc.playwrightInstalled {
				// Create temporary directory with playwright drivers.
				tmpDir := t.TempDir()
				homeDir := tmpDir
				t.Setenv("HOME", homeDir)
				t.Setenv("USERPROFILE", homeDir)

				// Clear homedir cache to ensure environment variables take effect.
				t.Cleanup(homedir.Reset)
				homedir.Reset()

				// Create a mock playwright driver directory with a file inside to pass validation.
				playwrightPath := filepath.Join(homeDir, ".cache", "ms-playwright", "chromium-1084")
				err := os.MkdirAll(playwrightPath, 0o755)
				require.NoError(t, err)

				// hasValidPlaywrightDrivers checks for files inside version directory.
				dummyBinary := filepath.Join(playwrightPath, "chrome")
				err = os.WriteFile(dummyBinary, []byte("dummy"), 0o755)
				require.NoError(t, err)
			} else {
				// Use empty temp directory (no drivers).
				tmpDir := t.TempDir()
				t.Setenv("HOME", tmpDir)
				t.Setenv("USERPROFILE", tmpDir)

				// Clear homedir cache to ensure environment variables take effect.
				t.Cleanup(homedir.Reset)
				homedir.Reset()
			}

			result := sp.shouldDownloadBrowser()
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestSAMLProvider_Logout(t *testing.T) {
	tests := []struct {
		name        string
		providerCfg *schema.Provider
		expectError bool
	}{
		{
			name: "successful logout",
			providerCfg: &schema.Provider{
				Kind:   "aws/saml",
				URL:    "https://idp.example.com/saml",
				Region: "us-east-1",
			},
			expectError: false,
		},
		{
			name: "logout with custom base_path",
			providerCfg: &schema.Provider{
				Kind:   "aws/saml",
				URL:    "https://idp.example.com/saml",
				Region: "us-east-1",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": t.TempDir(),
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewSAMLProvider("test-saml", tt.providerCfg)
			require.NoError(t, err)

			testProviderLogoutWithFilesystemVerification(t, tt.providerCfg, "test-saml", p, tt.expectError)
		})
	}
}

func TestSAMLProvider_Validate_URLFormats(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		expectErr bool
	}{
		{
			name:      "valid https URL",
			url:       "https://idp.example.com/saml",
			expectErr: false,
		},
		{
			name:      "valid http URL (not recommended but valid)",
			url:       "http://idp.example.com/saml",
			expectErr: false,
		},
		{
			name:      "invalid URL missing scheme",
			url:       "idp.example.com/saml",
			expectErr: true,
		},
		{
			name:      "invalid URL malformed",
			url:       "://bad",
			expectErr: true,
		},
		{
			name:      "invalid URL with spaces",
			url:       "https://idp example.com/saml",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewSAMLProvider("test", &schema.Provider{
				Kind:   "aws/saml",
				URL:    tc.url,
				Region: "us-east-1",
			})
			require.NoError(t, err)

			err = p.Validate()
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSAMLProvider_GetFilesDisplayPath(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.Provider
		expected string
	}{
		{
			name: "default path with no base_path",
			config: &schema.Provider{
				Kind:   "aws/saml",
				URL:    "https://idp.example.com/saml",
				Region: "us-east-1",
			},
			expected: "atmos/aws", // XDG path contains atmos/aws
		},
		{
			name: "custom base_path",
			config: &schema.Provider{
				Kind:   "aws/saml",
				URL:    "https://idp.example.com/saml",
				Region: "us-east-1",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "/custom/path",
					},
				},
			},
			expected: "/custom/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewSAMLProvider("test-saml", tt.config)
			require.NoError(t, err)

			path := provider.GetFilesDisplayPath()
			// Normalize path separators for cross-platform compatibility.
			normalizedPath := filepath.ToSlash(path)
			assert.Contains(t, normalizedPath, tt.expected)
		})
	}
}

func TestSAMLProvider_Logout_ErrorPaths(t *testing.T) {
	tests := []struct {
		name        string
		providerCfg *schema.Provider
		expectError bool
	}{
		{
			name: "handles invalid base_path gracefully",
			providerCfg: &schema.Provider{
				Kind:   "aws/saml",
				URL:    "https://idp.example.com/saml",
				Region: "us-east-1",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "/invalid/\x00/path", // Invalid path with null character.
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewSAMLProvider("test-saml", tt.providerCfg)
			require.NoError(t, err)

			ctx := context.Background()
			err = p.Logout(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSAMLProvider_Environment_AutoDownload(t *testing.T) {
	tests := []struct {
		name                    string
		downloadBrowserDriver   bool
		expectedAutoDownloadVar string
	}{
		{
			name:                    "download browser driver enabled",
			downloadBrowserDriver:   true,
			expectedAutoDownloadVar: "true",
		},
		{
			name:                    "download browser driver disabled",
			downloadBrowserDriver:   false,
			expectedAutoDownloadVar: "",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewSAMLProvider("test", &schema.Provider{
				Kind:                  "aws/saml",
				URL:                   "https://idp.example.com/saml",
				Region:                "us-west-2",
				DownloadBrowserDriver: tc.downloadBrowserDriver,
			})
			require.NoError(t, err)

			env, err := p.Environment()
			require.NoError(t, err)

			if tc.expectedAutoDownloadVar != "" {
				assert.Equal(t, tc.expectedAutoDownloadVar, env["SAML2AWS_AUTO_BROWSER_DOWNLOAD"])
			} else {
				_, exists := env["SAML2AWS_AUTO_BROWSER_DOWNLOAD"]
				assert.False(t, exists)
			}
		})
	}
}

func TestSAMLProvider_Paths(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	// Disable homedir cache to ensure our test environment is used.
	homedir.DisableCache = true
	t.Cleanup(func() { homedir.DisableCache = false })

	provider, err := NewSAMLProvider("test-saml", &schema.Provider{
		Kind:   "aws/saml",
		URL:    "https://idp.example.com/saml",
		Region: "us-west-2",
	})
	require.NoError(t, err)

	paths, err := provider.Paths()
	assert.NoError(t, err)
	assert.Len(t, paths, 3, "should return credentials, config, and cache paths")

	// Verify credentials file.
	assert.Equal(t, types.PathTypeFile, paths[0].Type)
	assert.True(t, paths[0].Required)
	assert.Contains(t, paths[0].Location, "credentials")
	assert.Equal(t, "true", paths[0].Metadata["read_only"])

	// Verify config file.
	assert.Equal(t, types.PathTypeFile, paths[1].Type)
	assert.False(t, paths[1].Required, "config file is optional")
	assert.Contains(t, paths[1].Location, "config")
	assert.Equal(t, "true", paths[1].Metadata["read_only"])

	// Verify cache directory.
	assert.Equal(t, types.PathTypeDirectory, paths[2].Type)
	assert.False(t, paths[2].Required, "cache is optional")
	assert.Contains(t, paths[2].Purpose, "cache")
	assert.Equal(t, "false", paths[2].Metadata["read_only"], "cache must be writable")
}
