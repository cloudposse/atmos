package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/ini.v1"
)

func TestAwsSaml_Validate(t *testing.T) {
	i := &awsSaml{}
	if err := i.Validate(); err == nil {
		t.Fatalf("expected error when url/profile missing")
	}
	i.Common.Url = "https://idp.example"
	if err := i.Validate(); err == nil {
		t.Fatalf("expected error when profile missing")
	}
	i.Common.Profile = "default"
	if err := i.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Ensure SetEnvVars writes config file with region, source_profile and role_arn for SAML delegation
func TestAwsSaml_SetEnvVars_WritesConfigWithDelegation(t *testing.T) {
	tmp := t.TempDir()
	cfgFile := filepath.Join(tmp, ".aws", "config")
	t.Setenv("HOME", tmp)
	t.Setenv("ATMOS_AWS_CONFIG_FILE", cfgFile)
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	i := &awsSaml{}
	i.Common.Region = "eu-central-1"
	i.Common.Profile = "src-prof"
	i.Identity.Identity = "saml-prof"
	i.Provider = "aws/saml"
	i.RoleArnToAssume = "arn:aws:iam::123456789012:role/SAMLRole"

	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{}}
	if err := i.SetEnvVars(info); err != nil {
		t.Fatalf("SetEnvVars: %v", err)
	}

	// Env
	if info.ComponentEnvSection["AWS_PROFILE"] != i.Identity.Identity {
		t.Fatalf("expected AWS_PROFILE %q, got %v", i.Identity.Identity, info.ComponentEnvSection["AWS_PROFILE"])
	}
	if info.ComponentEnvSection["AWS_REGION"] != i.Common.Region {
		t.Fatalf("expected AWS_REGION %q, got %v", i.Common.Region, info.ComponentEnvSection["AWS_REGION"])
	}

	// Config
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
	if !sec.HasKey("source_profile") || sec.Key("source_profile").String() != i.Common.Profile {
		t.Fatalf("expected source_profile %q to be set", i.Common.Profile)
	}
	if !sec.HasKey("role_arn") || sec.Key("role_arn").String() != i.RoleArnToAssume {
		t.Fatalf("expected role_arn %q to be set", i.RoleArnToAssume)
	}
}

// Logout should remove the credentials profile from the shared credentials file
func TestAwsSaml_Logout_RemovesCredentials(t *testing.T) {
	tmp := t.TempDir()
	credFile := filepath.Join(tmp, ".aws", "credentials")
	if err := os.MkdirAll(filepath.Dir(credFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)

	// Seed a credentials file with the target profile
	f := ini.Empty()
	sec := f.Section("my-src-profile")
	sec.Key("aws_access_key_id").SetValue("AKIA...")
	if err := f.SaveTo(credFile); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}

	i := &awsSaml{}
	i.Common.Profile = "my-src-profile"

	if err := i.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Verify profile section removed
	f2, err := ini.Load(credFile)
	if err != nil {
		t.Fatalf("reload credentials: %v", err)
	}
	if f2.HasSection("my-src-profile") {
		t.Fatalf("expected profile to be removed from credentials")
	}
}

// AssumeRole without a prior Login should return an error
func TestAwsSaml_AssumeRole_WithoutLogin_ReturnsError(t *testing.T) {
	i := &awsSaml{}
	if err := i.AssumeRole(); err == nil {
		t.Fatalf("expected error when no SAML assertion available")
	}
}

func TestEnsureSaml2awsStorageDir(t *testing.T) {
	// Use temp HOME to avoid touching real ~/.aws/saml2aws
	dir := t.TempDir()
	old := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	defer os.Setenv("HOME", old)

	if err := ensureSaml2awsStorageDir(); err != nil {
		t.Fatalf("ensureSaml2awsStorageDir error: %v", err)
	}
	p := filepath.Join(dir, ".aws", "saml2aws")
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		t.Fatalf("expected directory created at %s, err=%v", p, err)
	}
}

func TestAwsSaml_SetEnvVarsDelegates(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{}}
	i := &awsSaml{}
	i.Common.Profile = "p"    // This is the source profile
	i.Identity.Identity = "i" // This is the new profile created on the fly.
	i.Common.Region = "us-west-2"

	wantedProfile := "i"
	i.Provider = "idp"
	if err := i.SetEnvVars(info); err != nil {
		t.Fatalf("SetEnvVars error: %v", err)
	}
	if got := info.ComponentEnvSection["AWS_PROFILE"]; got != wantedProfile {
		t.Fatalf("expected AWS_PROFILE '%s', got %v", wantedProfile, got)
	}
	if got := info.ComponentEnvSection["AWS_REGION"]; got != "us-west-2" {
		t.Fatalf("expected AWS_REGION 'us-west-2', got %v", got)
	}
}
