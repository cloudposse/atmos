package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ini "gopkg.in/ini.v1"
)

// changeWorkingDir mirrors the helper used in pkg/config/config_test.go
func changeWorkingDir(t *testing.T, dir string) {
	cwd, err := os.Getwd()
	require.NoError(t, err, "Failed to get current directory")

	t.Cleanup(func() {
		err := os.Chdir(cwd)
		require.NoError(t, err, "Failed to restore working directory")
	})

	err = os.Chdir(dir)
	require.NoError(t, err, "Failed to change working directory")
}

// startMockSTS spins up a minimal STS endpoint that responds to AssumeRoleWithWebIdentity
// with a fixed XML response the AWS SDK can parse.
func startMockSTS(t *testing.T) *httptest.Server {
	t.Helper()
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Validate Action for clarity (not strictly required)
		_ = r.ParseForm()
		action := r.Form.Get("Action")
		if action != "AssumeRoleWithWebIdentity" {
			http.Error(w, "Invalid Action", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		// Minimal valid STS XML
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>ASIAEXAMPLE</AccessKeyId>
      <SecretAccessKey>secret</SecretAccessKey>
      <SessionToken>token</SessionToken>
      <Expiration>2030-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
  <ResponseMetadata>
    <RequestId>00000000-0000-0000-0000-000000000000</RequestId>
  </ResponseMetadata>
</AssumeRoleWithWebIdentityResponse>`)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// TestScenario_Auth_OIDC_GetIdentity loads the atmos-auth scenario fixtures and verifies
// the auth package can resolve the configured OIDC identity and its provider defaults
// without performing any network operations.
func TestScenario_Auth_OIDC_GetIdentity(t *testing.T) {
	// Use committed scenario fixtures
	scenarioDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-auth")
	changeWorkingDir(t, scenarioDir)

	// Initialize Atmos config from the scenario
	atmosCfg, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Sanity-check the scenario loaded as expected
	assert.Equal(t, "./", atmosCfg.BasePath)
	// Providers and identities should be present per tests/fixtures/scenarios/atmos-auth/atmos.yaml
	require.NotNil(t, atmosCfg.Auth.Providers)
	require.NotNil(t, atmosCfg.Auth.Identities)

	// The identity key is "oidc" and its provider is "oidcprov" of type aws/oidc
	idpName, err := GetIdp("oidc", atmosCfg.Auth)
	require.NoError(t, err)
	assert.Equal(t, "oidcprov", idpName)

	typeVal, err := GetType(idpName, atmosCfg.Auth)
	require.NoError(t, err)
	assert.Equal(t, "aws/oidc", typeVal)

	// Ensure we can construct the concrete identity instance and it has defaults from provider config
	lm, err := GetIdentityInstance("oidc", atmosCfg.Auth, nil)
	require.NoError(t, err)
	awsOidcInst, ok := lm.(*awsOidc)
	if !ok {
		t.Fatalf("expected *awsOidc, got %T", lm)
	}

	// Validate required fields populated from scenario
	require.NoError(t, awsOidcInst.Validate())
	assert.Equal(t, "arn:aws:iam::000000000000:role/Dummy", awsOidcInst.RoleArn)
	assert.Equal(t, "us-east-1", awsOidcInst.Common.Region)
	assert.Equal(t, "oidc-prof", awsOidcInst.Common.Profile)
}

// TestScenario_Auth_OIDC_Login_WithMockSTS validates that Login() exchanges a token using
// a mocked STS endpoint and writes credentials to the configured credentials file.
func TestScenario_Auth_OIDC_Login_WithMockSTS(t *testing.T) {
	scenarioDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-auth")
	changeWorkingDir(t, scenarioDir)

	atmosCfg, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	lm, err := GetIdentityInstance("oidc", atmosCfg.Auth, nil)
	require.NoError(t, err)
	i, ok := lm.(*awsOidc)
	if !ok {
		t.Fatalf("expected *awsOidc, got %T", lm)
	}

	// Provide a fake OIDC token via temp file to avoid network calls
	jwtFile := filepath.Join(t.TempDir(), "token.jwt")
	require.NoError(t, os.WriteFile(jwtFile, []byte("header.payload.signature"), 0o600))
	i.ForceTokenFile = jwtFile

	// Mock STS server and point the client at it
	srv := startMockSTS(t)
	i.STSEndpoint = srv.URL

	// Write credentials to a temp path instead of the real ~/.aws/credentials
	credsPath := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)

	require.NoError(t, i.Validate())
	require.NoError(t, i.Login())

	// Verify credentials were written by parsing as INI
	f, err := ini.Load(credsPath)
	require.NoError(t, err)
	sec := f.Section("oidc-prof")
	require.NotNil(t, sec)
	assert.Equal(t, "ASIAEXAMPLE", sec.Key("aws_access_key_id").String())
	assert.Equal(t, "secret", sec.Key("aws_secret_access_key").String())
	assert.Equal(t, "token", sec.Key("aws_session_token").String())
}

// TestScenario_Auth_OIDC_SetEnvVars verifies SetEnvVars populates env map and writes
// an AWS config with region set and no role_arn chaining for OIDC flow.
func TestScenario_Auth_OIDC_SetEnvVars(t *testing.T) {
	scenarioDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-auth")
	changeWorkingDir(t, scenarioDir)

	atmosCfg, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	lm, err := GetIdentityInstance("oidc", atmosCfg.Auth, nil)
	require.NoError(t, err)
	i, ok := lm.(*awsOidc)
	if !ok {
		t.Fatalf("expected *awsOidc, got %T", lm)
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentEnvSection: make(schema.AtmosSectionMapType),
	}

	// Direct SetEnvVars should create an atmos AWS config file under a temp HOME if ATMOS_AWS_CONFIG_FILE not set
	// Force the config path to a temp file for determinism
	cfgFile := filepath.Join(t.TempDir(), "config")
	t.Setenv("ATMOS_AWS_CONFIG_FILE", cfgFile)

	require.NoError(t, i.SetEnvVars(info))

	// Env should include AWS_PROFILE and AWS_REGION
	assert.Equal(t, "oidc-prof", info.ComponentEnvSection["AWS_PROFILE"]) // from scenario provider profile
	assert.Equal(t, "us-east-1", info.ComponentEnvSection["AWS_REGION"])   // from scenario provider region
	// AWS_CONFIG_FILE should be set to our forced path
	assert.Equal(t, cfgFile, info.ComponentEnvSection["AWS_CONFIG_FILE"])

	// Validate config file contents contain region and do NOT contain role_arn
	b, err := os.ReadFile(cfgFile)
	require.NoError(t, err)
	s := string(b)
	assert.Contains(t, s, "[profile oidc-prof]")
	assert.Contains(t, s, "region = us-east-1")
	assert.NotContains(t, s, "role_arn")
}
