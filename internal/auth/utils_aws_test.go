package auth

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetAwsAtmosConfigFilepath(t *testing.T) {
	type args struct {
		provider string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CreateAwsAtmosConfigFilepath(tt.args.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateAwsAtmosConfigFilepath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CreateAwsAtmosConfigFilepath() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAwsCredentialsFilepath(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetAwsCredentialsFilepath()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAwsCredentialsFilepath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetAwsCredentialsFilepath() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveAwsCredentials(t *testing.T) {
	type args struct {
		profile string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := RemoveAwsCredentials(tt.args.profile); (err != nil) != tt.wantErr {
				t.Errorf("RemoveAwsCredentials() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetAwsEnvVars(t *testing.T) {
	type args struct {
		info     *schema.ConfigAndStacksInfo
		profile  string
		provider string
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
			},
			wantErr: true,
		},
		{
			name: "InValid Configuration ProviderName NOT set",
			args: args{
				profile: "foo",
				info: &schema.ConfigAndStacksInfo{
					ComponentEnvSection: schema.AtmosSectionMapType{},
				},
				provider: "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SetAwsEnvVars(tt.args.info, tt.args.profile, tt.args.provider); (err != nil) && !tt.wantErr {
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

			// Test Config File Env Var
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
	type args struct {
		profile         string
		accessKeyID     string
		secretAccessKey string
		sessionToken    string
		identity        string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := WriteAwsCredentials(tt.args.profile, tt.args.accessKeyID, tt.args.secretAccessKey, tt.args.sessionToken, tt.args.identity); (err != nil) != tt.wantErr {
				t.Errorf("WriteAwsCredentials() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
