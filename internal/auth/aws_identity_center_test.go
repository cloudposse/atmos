package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ini "gopkg.in/ini.v1"
)

func TestRoleToAccountId(t *testing.T) {
	arn := "arn:aws:iam::123456789012:role/Admin"
	got := RoleToAccountId(arn)
	if got != "123456789012" {
		t.Fatalf("expected account id 123456789012, got %s", got)
	}
}

// startMockSTSGetCallerIdentity serves a minimal XML response for STS GetCallerIdentity
func startMockSTSGetCallerIdentity(t *testing.T) *httptest.Server {
	t.Helper()
	h := http.NewServeMux()
	h.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("Action") != "GetCallerIdentity" {
			http.Error(w, "Invalid Action", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:sts::111122223333:assumed-role/Admin/session</Arn>
    <UserId>AROAXXXXX:session</UserId>
    <Account>111122223333</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata>
    <RequestId>00000000-0000-0000-0000-000000000000</RequestId>
  </ResponseMetadata>
</GetCallerIdentityResponse>`))
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// Test that credentials written by our helpers can be used by AWS SDK v2 against a mocked STS endpoint.
func TestIdentityCenter_CallerIdentity_WithMockSTS(t *testing.T) {
	// Write credentials to a temp file
	credsPath := filepath.Join(t.TempDir(), "credentials")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)

	// Simulate Identity Center AssumeRole output by writing static creds for a profile
	err := WriteAwsCredentials("sso-prof", "AKIATEST", "secret", "token", "aws/iam-identity-center")
	require.NoError(t, err)

	// Mock STS
	srv := startMockSTSGetCallerIdentity(t)

	// Load config from that profile and set STS base endpoint to mock
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithSharedConfigProfile("sso-prof"),
	)
	require.NoError(t, err)
	stsClient := sts.NewFromConfig(cfg, func(o *sts.Options) { o.BaseEndpoint = aws.String(srv.URL) })

	out, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	require.NoError(t, err)
	assert.Equal(t, "111122223333", aws.ToString(out.Account))
}

// Test SetEnvVars for Identity Center: ensures env gets set and config contains region & role_arn chaining
func TestIdentityCenter_SetEnvVars_WritesConfig(t *testing.T) {
	// Construct a minimal identity center instance without hitting network
	ic := &awsIamIdentityCenter{
		Common: schema.ProviderDefaultConfig{
			Region:  "us-west-2",
			Profile: "sso-prof",
		},
		Identity: schema.Identity{
			Identity:        "ic", // identity key
			Provider:        "aws/iam-identity-center",
			RoleArnToAssume: "arn:aws:iam::111122223333:role/Admin",
		},
		RoleName:  "Admin",
		AccountId: "111122223333",
	}

	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: make(schema.AtmosSectionMapType)}

	// Force AWS config file path to a temp file
	cfgFile := filepath.Join(t.TempDir(), "config")
	t.Setenv("ATMOS_AWS_CONFIG_FILE", cfgFile)

	require.NoError(t, ic.SetEnvVars(info))

	// Env should have profile/region/config path
	assert.Equal(t, "ic", info.ComponentEnvSection["AWS_PROFILE"]) // CreateAwsFilesAndUpdateEnvVars uses identity as profile for IC
	assert.Equal(t, "us-west-2", info.ComponentEnvSection["AWS_REGION"])
	assert.Equal(t, cfgFile, info.ComponentEnvSection["AWS_CONFIG_FILE"])

	// Config file should include region, source_profile, and role_arn in [profile ic]
	f, err := ini.Load(cfgFile)
	require.NoError(t, err)
	sec := f.Section("profile ic")
	require.NotNil(t, sec)
	assert.Equal(t, "us-west-2", sec.Key("region").String())
	assert.Equal(t, "sso-prof", sec.Key("source_profile").String())
	assert.Equal(t, "arn:aws:iam::111122223333:role/Admin", sec.Key("role_arn").String())
}
