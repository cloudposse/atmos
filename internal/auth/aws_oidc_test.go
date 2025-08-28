package auth

import (
	"os"
	"path/filepath"
	"net/http"
	"net/http/httptest"
	"io"
	"strings"
	"testing"
	"time"
	"github.com/cloudposse/atmos/pkg/schema"
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
	i.RoleArn = "arn:aws:iam::111111111111:role/Test"
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
