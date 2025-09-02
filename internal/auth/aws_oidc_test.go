package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/cloudposse/atmos/pkg/schema"
	ini "gopkg.in/ini.v1"
)

func TestAwsOidc_Validate_Login_AssumeRole_SetEnv(t *testing.T) {
	// Isolate HOME and AWS files
	tmp := t.TempDir()
	credFile := filepath.Join(tmp, ".aws", "credentials")
	cfgFile := filepath.Join(tmp, ".aws", "config")
	t.Setenv("HOME", tmp)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)
	t.Setenv("AWS_CONFIG_FILE", cfgFile)

	// STS stub server for AssumeRoleWithWebIdentity used in Login()
	stsStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("Action")
		if action == "" {
			action = r.URL.Query().Get("Action")
		}

		if strings.ToLower(action) != "assumerolewithwebidentity" {
			http.Error(w, "unsupported action", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>ASIAFAKEKEY</AccessKeyId>
      <SecretAccessKey>fake_secret_key</SecretAccessKey>
      <SessionToken>fake_session_token</SessionToken>
      <Expiration>2030-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
  <ResponseMetadata><RequestId>req-123</RequestId></ResponseMetadata>
</AssumeRoleWithWebIdentityResponse>`)
	}))
	defer stsStub.Close()

	// Prepare token file
	tokenPath := filepath.Join(tmp, "token.jwt")
	if err := os.WriteFile(tokenPath, []byte("header.payload.signature"), 0o644); err != nil {
		t.Fatalf("write token: %v", err)
	}

	i := &awsOidc{}
	i.Common.Region = "us-east-1"
	i.Common.Profile = "src-prof"       // source profile name used for credentials file section
	i.Identity.Identity = "ci-identity" // identity name -> becomes AWS_PROFILE in env
	i.Provider = "aws/oidc"
	i.RoleArnToAssume = "arn:aws:iam::111111111111:role/Test"
	i.ForceTokenFile = tokenPath
	i.STSEndpoint = stsStub.URL
	i.RequestedDuration = 900 * time.Second

	if err := i.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := i.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}

	// AssumeRole is a no-op in the OIDC flow; credentials are written in Login().
	if err := i.AssumeRole(); err != nil {
		t.Fatalf("AssumeRole (noop) returned error: %v", err)
	}

	// Verify credentials written
	b, err := os.ReadFile(credFile)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "["+i.Common.Profile+"]") || !strings.Contains(content, "aws_session_token") {
		t.Fatalf("expected credentials for profile %q written, got:\n%s", i.Common.Profile, content)
	}

	// Verify env setup writes AWS_PROFILE = Common.Profile (credentials profile)
	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{}}
	if err := i.SetEnvVars(info); err != nil {
		t.Fatalf("SetEnvVars: %v", err)
	}
	if got := info.ComponentEnvSection["AWS_PROFILE"]; got != i.Common.Profile {
		t.Fatalf("expected AWS_PROFILE %q, got %v", i.Common.Profile, got)
	}
	if got := info.ComponentEnvSection["AWS_REGION"]; got != i.Common.Region {
		t.Fatalf("expected AWS_REGION %q, got %v", i.Common.Region, got)
	}
}

// startMockSTSGetCallerIdentity serves a minimal XML response for STS GetCallerIdentity
func startMockSTSGetCallerIdentity_OIDC(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("Action") != "GetCallerIdentity" {
			http.Error(w, "Invalid Action", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:sts::222233334444:assumed-role/Test/session</Arn>
    <UserId>AROATEST:session</UserId>
    <Account>222233334444</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata><RequestId>req-123</RequestId></ResponseMetadata>
</GetCallerIdentityResponse>`)
	}))
}

// After Login writes credentials, verify we can call GetCallerIdentity using those creds against a mock STS.
func TestAwsOidc_Login_ThenCallerIdentity_WithMockSTS(t *testing.T) {
	tmp := t.TempDir()
	credFile := filepath.Join(tmp, ".aws", "credentials")
	cfgFile := filepath.Join(tmp, ".aws", "config")
	t.Setenv("HOME", tmp)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)
	t.Setenv("AWS_CONFIG_FILE", cfgFile)

	// Stub AssumeRoleWithWebIdentity for Login()
	stsStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if strings.ToLower(r.FormValue("Action")) != "assumerolewithwebidentity" {
			http.Error(w, "unsupported action", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>ASIAFAKEKEY2</AccessKeyId>
      <SecretAccessKey>fake_secret_key_2</SecretAccessKey>
      <SessionToken>fake_session_token_2</SessionToken>
      <Expiration>2030-01-01T00:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
  <ResponseMetadata><RequestId>req-abc</RequestId></ResponseMetadata>
</AssumeRoleWithWebIdentityResponse>`)
	}))
	defer stsStub.Close()

	// Prepare token file
	tokenPath := filepath.Join(tmp, "token.jwt")
	if err := os.WriteFile(tokenPath, []byte("header.payload.signature"), 0o644); err != nil {
		t.Fatalf("write token: %v", err)
	}

	i := &awsOidc{}
	i.Common.Region = "us-east-1"
	i.Common.Profile = "src-prof"
	i.Identity.Identity = "ci-identity"
	i.Provider = "aws/oidc"
	i.RoleArnToAssume = "arn:aws:iam::222233334444:role/Test"
	i.ForceTokenFile = tokenPath
	i.STSEndpoint = stsStub.URL
	i.RequestedDuration = 900 * time.Second

	if err := i.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := i.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Now verify a GetCallerIdentity call with the written creds works against a mocked STS
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(i.Common.Region),
		config.WithSharedConfigProfile(i.Common.Profile),
	)
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}
	stsIdentityMock := startMockSTSGetCallerIdentity_OIDC(t)
	defer stsIdentityMock.Close()
	stsClient := sts.NewFromConfig(cfg, func(o *sts.Options) { o.BaseEndpoint = aws.String(stsIdentityMock.URL) })
	out, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Fatalf("GetCallerIdentity: %v", err)
	}
	if aws.ToString(out.Account) != "222233334444" {
		t.Fatalf("expected account 222233334444, got %s", aws.ToString(out.Account))
	}
}

// Ensure SetEnvVars writes minimal config for OIDC (no role_arn, no source_profile)
func TestAwsOidc_SetEnvVars_WritesMinimalConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgFile := filepath.Join(tmp, ".aws", "config")
	t.Setenv("HOME", tmp)
	t.Setenv("ATMOS_AWS_CONFIG_FILE", cfgFile)
	// Ensure directory exists for UpdateAwsAtmosConfig atomic write
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	i := &awsOidc{}
	i.Common.Region = "eu-west-1"
	i.Common.Profile = "ci-prof"
	i.Identity.Identity = "ci-identity"
	i.Provider = "aws/oidc"
	i.RoleArnToAssume = "arn:aws:iam::999988887777:role/Test"

	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{}}
	if err := i.SetEnvVars(info); err != nil {
		t.Fatalf("SetEnvVars: %v", err)
	}
	// env
	if info.ComponentEnvSection["AWS_PROFILE"] != i.Common.Profile {
		t.Fatalf("expected AWS_PROFILE %q, got %v", i.Common.Profile, info.ComponentEnvSection["AWS_PROFILE"])
	}
	if info.ComponentEnvSection["AWS_REGION"] != i.Common.Region {
		t.Fatalf("expected AWS_REGION %q, got %v", i.Common.Region, info.ComponentEnvSection["AWS_REGION"])
	}

	// config
	f, err := ini.Load(cfgFile)
	if err != nil {
		t.Fatalf("ini load: %v", err)
	}
	sec := f.Section("profile " + i.Common.Profile)
	if sec == nil {
		t.Fatalf("missing profile section")
	}
	if sec.Key("region").String() != i.Common.Region {
		t.Fatalf("expected region %q, got %q", i.Common.Region, sec.Key("region").String())
	}
	if sec.HasKey("role_arn") {
		t.Fatalf("did not expect role_arn in OIDC config")
	}
	if sec.HasKey("source_profile") {
		t.Fatalf("did not expect source_profile in OIDC config")
	}
}
