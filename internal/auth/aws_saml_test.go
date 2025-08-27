package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
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
