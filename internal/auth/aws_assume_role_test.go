package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	ini "gopkg.in/ini.v1"
)

func TestAwsAssumeRole_Validate_DefaultsAndErrors(t *testing.T) {
	i := &awsAssumeRole{}
	// Missing role_arn -> error
	if err := i.Validate(); err == nil {
		t.Fatalf("expected error without role_arn")
	}
	// Provide role_arn, expect default region applied
	i.RoleArnToAssume = "arn:aws:iam::123456789012:role/MyRole"
	if err := i.Validate(); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	if i.Common.Region != "us-east-1" {
		t.Fatalf("expected default region us-east-1, got %q", i.Common.Region)
	}
}

// minimal STS stub that serves Query API XML for GetCallerIdentity and AssumeRole
func newMockStsServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.Form.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		switch action {
		case "GetCallerIdentity":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
				<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
				  <GetCallerIdentityResult>
				    <UserId>ABCD</UserId>
				    <Account>123456789012</Account>
				    <Arn>arn:aws:iam::123456789012:user/test</Arn>
				  </GetCallerIdentityResult>
				  <ResponseMetadata><RequestId>req</RequestId></ResponseMetadata>
				</GetCallerIdentityResponse>`))
		case "AssumeRole":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
				<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
				  <AssumeRoleResult>
				    <Credentials>
				      <AccessKeyId>ASIAFAKE</AccessKeyId>
				      <SecretAccessKey>secret</SecretAccessKey>
				      <SessionToken>token</SessionToken>
				      <Expiration>2030-01-01T00:00:00Z</Expiration>
				    </Credentials>
				  </AssumeRoleResult>
				  <ResponseMetadata><RequestId>req</RequestId></ResponseMetadata>
				</AssumeRoleResponse>`))
		default:
			w.WriteHeader(400)
		}
	}))
}

// server that always returns 400
func newErrorStsServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
	}))
}

func TestAwsAssumeRole_Login_Error_STS(t *testing.T) {
	srv := newErrorStsServer()
	defer srv.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAFAKE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	i := &awsAssumeRole{}
	i.Common.Region = "us-east-1"
	i.STSEndpoint = srv.URL
	if err := i.Login(); err == nil {
		t.Fatalf("expected error from STS, got nil")
	}
}

func TestAwsAssumeRole_Login_ValidatesWithSTS(t *testing.T) {
	srv := newMockStsServer()
	defer srv.Close()

	// Provide env credentials so SDK can sign requests
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAFAKE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	i := &awsAssumeRole{}
	i.Common.Region = "us-east-1"
	i.STSEndpoint = srv.URL

	if err := i.Login(); err != nil {
		t.Fatalf("Login: %v", err)
	}
}

func TestAwsAssumeRole_AssumeRole_WritesCredentials(t *testing.T) {
	tmp := t.TempDir()
	credFile := filepath.Join(tmp, ".aws", "credentials")
	cfgFile := filepath.Join(tmp, ".aws", "config")
	t.Setenv("HOME", tmp)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)
	t.Setenv("AWS_CONFIG_FILE", cfgFile)
	// SDK creds
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAFAKE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	srv := newMockStsServer()
	defer srv.Close()

	// Create minimal shared config with the source profile
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f := ini.Empty()
	sec := f.Section("profile src-prof")
	sec.Key("region").SetValue("us-east-1")
	if err := f.SaveTo(cfgFile); err != nil {
		t.Fatalf("save cfg: %v", err)
	}

	// Seed credentials in shared credentials file for src-prof
	if err := os.MkdirAll(filepath.Dir(credFile), 0o755); err != nil {
		t.Fatalf("mkdir creds: %v", err)
	}
	if err := WriteAwsCredentials("src-prof", "AKIAFAKE", "secret", "", "seed"); err != nil {
		t.Fatalf("seed creds: %v", err)
	}

	i := &awsAssumeRole{}
	i.Common.Profile = "src-prof"
	i.Common.Region = "us-east-1"
	i.RoleArnToAssume = "arn:aws:iam::111122223333:role/Target"
	i.STSEndpoint = srv.URL

	if err := i.AssumeRole(); err != nil {
		t.Fatalf("AssumeRole: %v", err)
	}

	f, err := ini.Load(credFile)
	if err != nil {
		t.Fatalf("load creds: %v", err)
	}
	sec2 := f.Section("src-prof")
	if sec2.Key("aws_access_key_id").String() != "ASIAFAKE" {
		t.Fatalf("expected creds written for profile, got %q", sec2.Key("aws_access_key_id").String())
	}
	if sec2.Key("aws_session_token").String() != "token" {
		t.Fatalf("expected session token written")
	}
}

func TestAwsAssumeRole_SetEnvVars_WritesConfigWithRoleAndSourceProfile(t *testing.T) {
	tmp := t.TempDir()
	cfgFile := filepath.Join(tmp, ".aws", "config")
	credFile := filepath.Join(tmp, ".aws", "credentials")
	t.Setenv("HOME", tmp)
	t.Setenv("ATMOS_AWS_CONFIG_FILE", cfgFile)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)

	// Ensure directory exists for atomic config write in UpdateAwsAtmosConfig
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	i := &awsAssumeRole{}
	i.Provider = "aws/assume-role"
	i.Identity.Identity = "my-ident"
	i.Common.Profile = "src-prof"
	i.Common.Region = "eu-central-1"
	i.RoleArnToAssume = "arn:aws:iam::111122223333:role/Target"

	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{}}
	if err := i.SetEnvVars(info); err != nil {
		t.Fatalf("SetEnvVars: %v", err)
	}

	// env vars written
	// For assume-role, AWS_PROFILE is set to identity (source profile name)
	if info.ComponentEnvSection["AWS_PROFILE"] != i.Identity.Identity {
		t.Fatalf("expected AWS_PROFILE %q, got %v", i.Identity.Identity, info.ComponentEnvSection["AWS_PROFILE"])
	}
	if info.ComponentEnvSection["AWS_REGION"] != i.Common.Region {
		t.Fatalf("expected AWS_REGION %q, got %v", i.Common.Region, info.ComponentEnvSection["AWS_REGION"])
	}

	// config written: profile section (named after identity) has region, role_arn, source_profile
	f, err := ini.Load(cfgFile)
	if err != nil {
		t.Fatalf("ini load: %v", err)
	}
	sec := f.Section("profile " + i.Identity.Identity)
	if sec == nil {
		t.Fatalf("missing profile section")
	}
	if sec.Key("region").String() != i.Common.Region {
		t.Fatalf("expected region %q, got %q", i.Common.Region, sec.Key("region").String())
	}
	if sec.Key("role_arn").String() != i.RoleArnToAssume {
		t.Fatalf("expected role_arn %q, got %q", i.RoleArnToAssume, sec.Key("role_arn").String())
	}
	if sec.Key("source_profile").String() != i.Common.Profile {
		t.Fatalf("expected source_profile %q, got %q", i.Common.Profile, sec.Key("source_profile").String())
	}
}

func TestAwsAssumeRole_Logout_RemovesCredentials(t *testing.T) {
	tmp := t.TempDir()
	credFile := filepath.Join(tmp, ".aws", "credentials")
	t.Setenv("HOME", tmp)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)

	// Seed credentials for profile
	WriteAwsCredentials("assume-prof", "AKIA...", "secret", "token", "seed")

	i := &awsAssumeRole{}
	i.Common.Profile = "assume-prof"
	if err := i.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	f, err := ini.Load(credFile)
	if err != nil {
		t.Fatalf("load creds: %v", err)
	}
	if f.HasSection("assume-prof") {
		t.Fatalf("expected credentials section removed for assume-prof")
	}
}
