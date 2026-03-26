package ambient

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewAmbientIdentity(t *testing.T) {
	tests := []struct {
		name      string
		idName    string
		config    *schema.Identity
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "valid ambient identity",
			idName: "passthrough",
			config: &schema.Identity{Kind: "ambient"},
		},
		{
			name:      "nil config",
			idName:    "bad",
			config:    nil,
			wantErr:   true,
			errSubstr: "nil config",
		},
		{
			name:      "wrong kind",
			idName:    "bad",
			config:    &schema.Identity{Kind: "aws/user"},
			wantErr:   true,
			errSubstr: "invalid identity kind",
		},
		{
			name:   "via is rejected",
			idName: "bad",
			config: &schema.Identity{
				Kind: "ambient",
				Via:  &schema.IdentityVia{Identity: "base-identity"},
			},
			wantErr:   true,
			errSubstr: "must not define via",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := NewAmbientIdentity(tt.idName, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				assert.Nil(t, identity)
			} else {
				require.NoError(t, err)
				require.NotNil(t, identity)
			}
		})
	}
}

func TestAmbientIdentityKind(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)
	assert.Equal(t, "ambient", identity.Kind())
}

func TestAmbientIdentityGetProviderName(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)
	name, err := identity.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "ambient", name)
}

func TestAmbientIdentityAuthenticate(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	creds, err := identity.Authenticate(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, creds, "ambient identity should return nil credentials")
}

func TestAmbientIdentityEnvironment(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	env, err := identity.Environment()
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestAmbientIdentityPaths(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	paths, err := identity.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestAmbientIdentityPrepareEnvironment(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	input := map[string]string{
		"AWS_ACCESS_KEY_ID":         "AKIAIOSFODNN7EXAMPLE",
		"AWS_SECRET_ACCESS_KEY":     "secret",
		"AWS_SESSION_TOKEN":         "token",
		"AWS_EC2_METADATA_DISABLED": "false",
		"CUSTOM_VAR":                "value",
	}

	result, err := identity.PrepareEnvironment(context.Background(), input)
	require.NoError(t, err)

	// All vars should be preserved — ambient does not clear anything.
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", result["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "secret", result["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "token", result["AWS_SESSION_TOKEN"])
	assert.Equal(t, "false", result["AWS_EC2_METADATA_DISABLED"])
	assert.Equal(t, "value", result["CUSTOM_VAR"])
}

func TestAmbientIdentityPrepareEnvironmentDoesNotMutateInput(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	input := map[string]string{
		"KEY": "original",
	}

	result, err := identity.PrepareEnvironment(context.Background(), input)
	require.NoError(t, err)

	// Mutate the result and verify input is unchanged.
	result["KEY"] = "modified"
	result["NEW_KEY"] = "new"

	assert.Equal(t, "original", input["KEY"], "input should not be mutated")
	_, exists := input["NEW_KEY"]
	assert.False(t, exists, "new keys should not appear in input")
}

func TestIsStandaloneAmbientChain(t *testing.T) {
	tests := []struct {
		name       string
		chain      []string
		identities map[string]schema.Identity
		want       bool
	}{
		{
			name:  "single ambient identity",
			chain: []string{"passthrough"},
			identities: map[string]schema.Identity{
				"passthrough": {Kind: "ambient"},
			},
			want: true,
		},
		{
			name:  "single non-ambient identity",
			chain: []string{"my-user"},
			identities: map[string]schema.Identity{
				"my-user": {Kind: "aws/user"},
			},
			want: false,
		},
		{
			name:  "multi-element chain",
			chain: []string{"deployer", "passthrough"},
			identities: map[string]schema.Identity{
				"passthrough": {Kind: "ambient"},
				"deployer":    {Kind: "aws/assume-role"},
			},
			want: false,
		},
		{
			name:       "empty chain",
			chain:      []string{},
			identities: map[string]schema.Identity{},
			want:       false,
		},
		{
			name:  "identity not found",
			chain: []string{"missing"},
			identities: map[string]schema.Identity{
				"other": {Kind: "ambient"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsStandaloneAmbientChain(tt.chain, tt.identities)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAmbientIdentityCredentialsExist(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	exists, err := identity.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestAmbientIdentityLoadCredentials(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	creds, err := identity.LoadCredentials(context.Background())
	require.NoError(t, err)
	assert.Nil(t, creds)
}

func TestAmbientIdentitySetRealm(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	// SetRealm is a no-op for ambient identities, just verify it doesn't panic.
	identity.SetRealm("test-realm")
}

func TestAmbientIdentityValidate(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	err = identity.Validate()
	assert.NoError(t, err)
}

func TestAmbientIdentityPostAuthenticate(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	err = identity.PostAuthenticate(context.Background(), nil)
	assert.NoError(t, err)
}

func TestAmbientIdentityLogout(t *testing.T) {
	identity, err := NewAmbientIdentity("test", &schema.Identity{Kind: "ambient"})
	require.NoError(t, err)

	err = identity.Logout(context.Background())
	assert.NoError(t, err)
}

// mockIdentity is a test double for types.Identity used by AuthenticateStandaloneAmbient tests.
type mockIdentity struct {
	kind      string
	authCreds types.ICredentials
	authErr   error
}

func (m *mockIdentity) Kind() string                            { return m.kind }
func (m *mockIdentity) GetProviderName() (string, error)        { return "mock", nil }
func (m *mockIdentity) Validate() error                         { return nil }
func (m *mockIdentity) Environment() (map[string]string, error) { return nil, nil }
func (m *mockIdentity) Paths() ([]types.Path, error)            { return nil, nil }
func (m *mockIdentity) SetRealm(_ string)                       {}
func (m *mockIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}
func (m *mockIdentity) Logout(_ context.Context) error  { return nil }
func (m *mockIdentity) CredentialsExist() (bool, error) { return true, nil }
func (m *mockIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, nil
}

func (m *mockIdentity) PrepareEnvironment(_ context.Context, env map[string]string) (map[string]string, error) {
	return env, nil
}

func (m *mockIdentity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	return m.authCreds, m.authErr
}

func TestAuthenticateStandaloneAmbient(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		identities   map[string]types.Identity
		wantErr      bool
		errSubstr    string
	}{
		{
			name:         "success",
			identityName: "passthrough",
			identities: map[string]types.Identity{
				"passthrough": &mockIdentity{kind: "ambient"},
			},
		},
		{
			name:         "identity not found",
			identityName: "missing",
			identities:   map[string]types.Identity{},
			wantErr:      true,
			errSubstr:    "not found",
		},
		{
			name:         "authentication fails",
			identityName: "broken",
			identities: map[string]types.Identity{
				"broken": &mockIdentity{
					kind:    "ambient",
					authErr: fmt.Errorf("credential resolution failed"),
				},
			},
			wantErr:   true,
			errSubstr: "authentication failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := AuthenticateStandaloneAmbient(context.Background(), tt.identityName, tt.identities)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
				assert.Nil(t, creds, "ambient identity should return nil credentials")
			}
		})
	}
}
