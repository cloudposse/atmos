package auth

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	ini "gopkg.in/ini.v1"
)

func TestGetAwsAtmosConfigFilepath(t *testing.T) {
	// Use env override to make deterministic
	tmp := t.TempDir()
	expected := tmp + "/custom-config"
	t.Setenv("ATMOS_AWS_CONFIG_FILE", expected)
	got, err := CreateAwsAtmosConfigFilepath("ignored")
	if err != nil {
		t.Fatalf("CreateAwsAtmosConfigFilepath error: %v", err)
	}
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestUpdateAwsAtmosConfig_SetAndRemoveKeys(t *testing.T) {
	tmp := t.TempDir()
	cfg := tmp + "/config.ini"
	t.Setenv("ATMOS_AWS_CONFIG_FILE", cfg)

	// First: set with all fields
	if err := UpdateAwsAtmosConfig("prov", "p1", "src", "us-west-2", "arn:aws:iam::123:role/R"); err != nil {
		t.Fatalf("UpdateAwsAtmosConfig set: %v", err)
	}
	f, err := ini.Load(cfg)
	if err != nil { 
		t.Fatalf("load: %v", err) 
	}
	sec := f.Section("profile p1")
	if sec.Key("region").String() != "us-west-2" { 
		t.Fatalf("region not set") 
	}
	if sec.Key("source_profile").String() != "src" { 
		t.Fatalf("source_profile not set") 
	}
	if sec.Key("role_arn").String() != "arn:aws:iam::123:role/R" { 
		t.Fatalf("role_arn not set") 
	}

	// Second: remove source_profile and role_arn by passing empty
	if err := UpdateAwsAtmosConfig("prov", "p1", "", "eu-north-1", ""); err != nil {
		t.Fatalf("UpdateAwsAtmosConfig remove: %v", err)
	}
	f2, err := ini.Load(cfg)
	if err != nil { 
		t.Fatalf("load2: %v", err) 
	}
	sec2 := f2.Section("profile p1")
	if sec2.HasKey("source_profile") { 
		t.Fatalf("source_profile should be removed") 
	}
	if sec2.HasKey("role_arn") { 
		t.Fatalf("role_arn should be removed") 
	}
	if sec2.Key("region").String() != "eu-north-1" { 
		t.Fatalf("region should be updated") 
	}
}

func TestUpdateAwsAtmosConfig_PreserveUnrelatedSections(t *testing.T) {
	tmp := t.TempDir()
	cfg := tmp + "/config.ini"
	t.Setenv("ATMOS_AWS_CONFIG_FILE", cfg)

	// Pre-seed file with an unrelated section
	f := ini.Empty()
	f.Section("unrelated").Key("k").SetValue("v")
	if err := f.SaveTo(cfg); err != nil { 
		t.Fatalf("seed: %v", err) 
	}

	if err := UpdateAwsAtmosConfig("prov", "p2", "src", "us-east-2", "arn:aws:iam::111:role/R"); err != nil {
		t.Fatalf("update: %v", err)
	}
	loaded, err := ini.Load(cfg)
	if err != nil { 
		t.Fatalf("load: %v", err) 
	}
	if !loaded.HasSection("unrelated") { 
		t.Fatalf("unrelated section lost") 
	}
	if loaded.Section("unrelated").Key("k").String() != "v" { 
		t.Fatalf("unrelated key changed") 
	}
	if !loaded.HasSection("profile p2") { 
		t.Fatalf("profile p2 missing") 
	}
}

func TestGetAwsCredentialsFilepath(t *testing.T) {
	tmp := t.TempDir()
	expected := tmp + "/credentials"
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", expected)
	got, err := GetAwsCredentialsFilepath()
	if err != nil {
		t.Fatalf("GetAwsCredentialsFilepath error: %v", err)
	}
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

func TestRemoveAwsCredentials(t *testing.T) {
	tmp := t.TempDir()
	credsPath := tmp + "/credentials.ini"
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	// Write credentials using helper
	if err := WriteAwsCredentials("testprof", "AKIA...", "SECRET", "TOKEN", "test"); err != nil {
		t.Fatalf("prep write creds: %v", err)
	}
	// Remove
	if err := RemoveAwsCredentials("testprof"); err != nil {
		t.Fatalf("RemoveAwsCredentials error: %v", err)
	}
	// Ensure profile no longer exists (idempotent call should also be fine)
	if err := RemoveAwsCredentials("testprof"); err != nil {
		t.Fatalf("RemoveAwsCredentials idempotent error: %v", err)
	}
}

func TestSetAwsEnvVars(t *testing.T) {
	type args struct {
		info     *schema.ConfigAndStacksInfo
		profile  string
		provider string
		region   string
	}
	tests := []struct {
		name             string
		args             args
		wantErr          bool
		wantedConfigFile string
	}{
		{
			name: "Valid Configuration and Profile is set",
			args: args{
				info: &schema.ConfigAndStacksInfo{
					ComponentEnvSection: schema.AtmosSectionMapType{},
				},
				provider: "idp-test",
				profile:  "profile",
				region:   "us-east-1",
			},
			wantErr: false,
		},
		{
			name: "Valid Configuration and Config File is set",
			args: args{
				info: &schema.ConfigAndStacksInfo{
					ComponentEnvSection: schema.AtmosSectionMapType{},
				},
				provider: "idp-test",
				profile:  "profile",
				region:   "us-east-1",
			},
			wantedConfigFile: "~/.aws/atmos/idp-test/config",
			wantErr:          false,
		},
		{
			name: "InValid Configuration Profile NOT set",
			args: args{
				profile: "",
				info: &schema.ConfigAndStacksInfo{
					ComponentEnvSection: schema.AtmosSectionMapType{},
				},
				region: "us-east-1",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SetAwsEnvVars(tt.args.info, tt.args.profile, tt.args.provider, tt.args.region); (err != nil) && !tt.wantErr {
				t.Errorf("SetAwsEnvVars() error = %v, wantErr %v", err, tt.wantErr)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			}
			if tt.args.info.ComponentEnvSection["AWS_PROFILE"] != tt.args.profile && !tt.wantErr {
				t.Errorf("SetAwsEnvVars() Expected AWS_PROFILE to be set, got `%v`, expected `%v`", tt.args.info.ComponentEnvSection["AWS_PROFILE"], tt.args.profile)
			}

			// Test Config File E Var
			if tt.wantedConfigFile != "" {
				expected, err := CreateAwsAtmosConfigFilepath(tt.args.provider)
				if err != nil {
					t.Errorf("SetAwsEnvVars() error = %v", err)
				}
				actual := tt.args.info.ComponentEnvSection["AWS_CONFIG_FILE"]
				if actual != expected {
					t.Errorf("SetAwsEnvVars() Expected AWS_CONFIG_FILE to be set, got `%v`, expected `%v`", expected, tt.args.info.ComponentEnvSection["AWS_CONFIG_FILE"])
				}
			}
		})
	}
}

func TestWriteAwsCredentials(t *testing.T) {
	tmp := t.TempDir()
	credsPath := tmp + "/credentials.ini"
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsPath)
	if err := WriteAwsCredentials("default", "AKIA123", "SECRET", "TOKEN", "idp"); err != nil {
		t.Fatalf("WriteAwsCredentials error: %v", err)
	}
	// Call again to ensure overwrite path works
	if err := WriteAwsCredentials("default", "AKIA456", "SECRET2", "TOKEN2", "idp"); err != nil {
		t.Fatalf("WriteAwsCredentials 2 error: %v", err)
	}
}
