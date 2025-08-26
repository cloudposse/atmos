package auth

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
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
