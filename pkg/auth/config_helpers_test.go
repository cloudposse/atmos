package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCopyGlobalAuthConfig(t *testing.T) {
	tests := []struct {
		name       string
		globalAuth *schema.AuthConfig
		verify     func(*testing.T, *schema.AuthConfig)
	}{
		{
			name:       "nil config returns empty",
			globalAuth: nil,
			verify: func(t *testing.T, result *schema.AuthConfig) {
				assert.NotNil(t, result)
				assert.Nil(t, result.Providers)
				assert.Nil(t, result.Identities)
			},
		},
		{
			name: "copies all fields",
			globalAuth: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind:   "aws/iam-identity-center",
						Region: "us-east-1",
					},
				},
				Identities: map[string]schema.Identity{
					"test-identity": {
						Kind:    "aws/user",
						Default: true,
					},
				},
				Logs: schema.Logs{
					Level: "Info",
				},
				Keyring: schema.KeyringConfig{
					Type: "system",
				},
				IdentityCaseMap: map[string]string{
					"test": "Test",
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig) {
				assert.Len(t, result.Providers, 1)
				assert.Contains(t, result.Providers, "test-provider")
				assert.Len(t, result.Identities, 1)
				assert.Contains(t, result.Identities, "test-identity")
				assert.Equal(t, "Info", result.Logs.Level)
				assert.Equal(t, "system", result.Keyring.Type)
				assert.Len(t, result.IdentityCaseMap, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CopyGlobalAuthConfig(tt.globalAuth)
			tt.verify(t, result)
		})
	}
}

func TestMergeComponentAuthFromConfig(t *testing.T) {
	globalAuth := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"global-provider": {
				Kind:   "aws/iam-identity-center",
				Region: "us-west-2",
			},
		},
		Identities: map[string]schema.Identity{
			"global-identity": {
				Kind:    "aws/user",
				Default: true,
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	tests := []struct {
		name            string
		componentConfig map[string]any
		verify          func(*testing.T, *schema.AuthConfig, error)
	}{
		{
			name:            "nil component config returns global",
			componentConfig: nil,
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.Len(t, result.Identities, 1)
				assert.Contains(t, result.Identities, "global-identity")
			},
		},
		{
			name: "no auth section returns global",
			componentConfig: map[string]any{
				"vars": map[string]any{
					"test": "value",
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.Len(t, result.Identities, 1)
				assert.Contains(t, result.Identities, "global-identity")
			},
		},
		{
			name: "merges component auth section",
			componentConfig: map[string]any{
				cfg.AuthSectionName: map[string]any{
					"identities": map[string]any{
						"component-identity": map[string]any{
							"kind": "aws/assume-role",
						},
					},
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.Len(t, result.Identities, 2)
				assert.Contains(t, result.Identities, "global-identity")
				assert.Contains(t, result.Identities, "component-identity")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MergeComponentAuthFromConfig(globalAuth, tt.componentConfig, atmosConfig, cfg.AuthSectionName)
			tt.verify(t, result, err)
		})
	}
}

func TestAuthConfigToMap(t *testing.T) {
	tests := []struct {
		name       string
		authConfig *schema.AuthConfig
		wantErr    bool
	}{
		{
			name:       "nil config returns empty map",
			authConfig: nil,
			wantErr:    false,
		},
		{
			name: "converts auth config to map",
			authConfig: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test": {Kind: "aws/user"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AuthConfigToMap(tt.authConfig)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
