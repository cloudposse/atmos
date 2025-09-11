package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_GetDefaultIdentity(t *testing.T) {
	tests := []struct {
		name            string
		identities      map[string]schema.Identity
		isCI            bool
		expectedResult  string
		expectedError   string
		skipInteractive bool // Skip tests that require user interaction.
	}{
		{
			name: "no default identities - CI mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: false},
				"identity2": {Kind: "aws/user", Default: false},
			},
			isCI:          true,
			expectedError: "no default identity configured",
		},
		{
			name: "no default identities - interactive mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: false},
				"identity2": {Kind: "aws/user", Default: false},
			},
			isCI:            false,
		skipInteractive: true, // Skip because it requires user interaction.
		},
		{
			name: "single default identity - CI mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: true},
				"identity2": {Kind: "aws/user", Default: false},
			},
			isCI:           true,
			expectedResult: "identity1",
		},
		{
			name: "single default identity - interactive mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: true},
				"identity2": {Kind: "aws/user", Default: false},
			},
			isCI:           false,
			expectedResult: "identity1",
		},
		{
			name: "multiple default identities - CI mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: true},
				"identity2": {Kind: "aws/user", Default: true},
				"identity3": {Kind: "aws/user", Default: false},
			},
			isCI:          true,
			expectedError: "multiple default identities found",
		},
		{
			name: "multiple default identities - interactive mode",
			identities: map[string]schema.Identity{
				"identity1": {Kind: "aws/user", Default: true},
				"identity2": {Kind: "aws/user", Default: true},
				"identity3": {Kind: "aws/user", Default: false},
			},
			isCI:            false,
		skipInteractive: true, // Skip because it requires user interaction.
		},
		{
			name:          "no identities at all - CI mode",
			identities:    map[string]schema.Identity{},
			isCI:          true,
			expectedError: "no default identity configured",
		},
		{
			name:            "no identities at all - interactive mode",
			identities:      map[string]schema.Identity{},
			isCI:            false,
		skipInteractive: true, // Skip because it requires user interaction.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipInteractive {
				t.Skipf("Skipping interactive test - requires user input.")
			}

			// Set up CI environment variable.
			originalCI := os.Getenv("CI")
			if tt.isCI {
				os.Setenv("CI", "true")
			} else {
				os.Unsetenv("CI")
			}
			defer func() {
				if originalCI != "" {
					os.Setenv("CI", originalCI)
				} else {
					os.Unsetenv("CI")
				}
			}()

			// Create manager with test identities.
			manager := &manager{
				config: &schema.AuthConfig{
					Identities: tt.identities,
				},
			}

			// Call the function.
			result, err := manager.GetDefaultIdentity()

			// Assert results.
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestManager_GetDefaultIdentity_MultipleDefaultsOrder(t *testing.T) {
	// Test that multiple defaults are returned in a consistent order.
	identities := map[string]schema.Identity{
		"zebra":   {Kind: "aws/user", Default: true},
		"alpha":   {Kind: "aws/user", Default: true},
		"beta":    {Kind: "aws/user", Default: false},
		"charlie": {Kind: "aws/user", Default: true},
	}

	// Set CI mode to get deterministic error message.
	os.Setenv("CI", "true")
	defer os.Unsetenv("CI")

	manager := &manager{
		config: &schema.AuthConfig{
			Identities: identities,
		},
	}

	_, err := manager.GetDefaultIdentity()
	require.Error(t, err)

	// The error should contain all three default identities.
	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "multiple default identities found:")
	assert.Contains(t, errorMsg, "alpha")
	assert.Contains(t, errorMsg, "charlie")
	assert.Contains(t, errorMsg, "zebra")
	// Should not contain the non-default identity.
	assert.NotContains(t, errorMsg, "beta")
}

func TestManager_ListIdentities(t *testing.T) {
	identities := map[string]schema.Identity{
		"identity1": {Kind: "aws/user", Default: true},
		"identity2": {Kind: "aws/user", Default: false},
		"identity3": {Kind: "aws/assume-role", Default: false},
	}

	manager := &manager{
		config: &schema.AuthConfig{
			Identities: identities,
		},
	}

	result := manager.ListIdentities()

	// Should return all identity names.
	assert.Len(t, result, 3)
	assert.Contains(t, result, "identity1")
	assert.Contains(t, result, "identity2")
	assert.Contains(t, result, "identity3")
}

func TestManager_promptForIdentity(t *testing.T) {
	manager := &manager{}

	// Test with empty identities list.
	_, err := manager.promptForIdentity("Choose identity:", []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no identities available")

	// Note: We can't easily test the interactive prompt without mocking huh.Form.
	// In a real test environment, you might want to use dependency injection.
	// To mock the form interaction.
}

// --- Additional helpers for manager tests ---.
type (
	testCreds struct{ exp *time.Time }
	testStore struct {
		data        map[string]any
		expired     map[string]bool
		retrieveErr map[string]error
	}
	testProvider struct {
		name    string
		kind    string
		creds   *testCreds
		authErr error
		preErr  error
	}
)

func (c *testCreds) IsExpired() bool {
	if c.exp == nil {
		return false
	}
	return time.Now().After(*c.exp)
}
func (c *testCreds) GetExpiration() (*time.Time, error)     { return c.exp, nil }
func (c *testCreds) BuildWhoamiInfo(info *types.WhoamiInfo) {}

func (s *testStore) Store(alias string, creds types.ICredentials) error {
	if s.data == nil {
		s.data = map[string]any{}
	}
	s.data[alias] = creds
	return nil
}

func (s *testStore) Retrieve(alias string) (types.ICredentials, error) {
	if s.retrieveErr != nil {
		if err, ok := s.retrieveErr[alias]; ok {
			return nil, err
		}
	}
	if s.data == nil {
		return nil, assert.AnError
	}
	v, ok := s.data[alias]
	if !ok {
		return nil, assert.AnError
	}
	return v.(types.ICredentials), nil
}
func (s *testStore) Delete(alias string) error { delete(s.data, alias); return nil }
func (s *testStore) List() ([]string, error)   { return nil, nil }
func (s *testStore) IsExpired(alias string) (bool, error) {
	if s.expired != nil {
		return s.expired[alias], nil
	}
	return false, nil
}

func (p *testProvider) Kind() string {
	if p.kind == "" {
		return "aws/iam-identity-center"
	}
	return p.kind
}
func (p *testProvider) Name() string                              { return p.name }
func (p *testProvider) PreAuthenticate(_ types.AuthManager) error { return p.preErr }
func (p *testProvider) Authenticate(_ context.Context) (types.ICredentials, error) {
	if p.authErr != nil {
		return nil, p.authErr
	}
	if p.creds == nil {
		return &testCreds{}, nil
	}
	return p.creds, nil
}
func (p *testProvider) Validate() error                         { return nil }
func (p *testProvider) Environment() (map[string]string, error) { return map[string]string{}, nil }

func TestManager_getProviderForIdentity_NameAndAlias(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{Identities: map[string]schema.Identity{
			"real": {Kind: "aws/permission-set", Alias: "alias"},
		}},
		identities: map[string]types.Identity{
			"real": stubIdentity{provider: "p"},
		},
	}
	assert.Equal(t, "p", m.getProviderForIdentity("real"))
	assert.Equal(t, "p", m.getProviderForIdentity("alias"))
	assert.Equal(t, "", m.getProviderForIdentity("missing"))
}

func TestManager_buildAuthenticationChain_Errors(t *testing.T) {
	m := &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{}}}
	_, err := m.buildAuthenticationChain("ghost")
	require.Error(t, err)

	m = &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{
		"dev": {Kind: "aws/permission-set"},
	}}}
	_, err = m.buildAuthenticationChain("dev")
	require.Error(t, err)

	m = &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{
		"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{}},
	}}}
	_, err = m.buildAuthenticationChain("dev")
	require.Error(t, err)

	m = &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{
		"a": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Identity: "b"}},
		"b": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Identity: "a"}},
	}}}
	_, err = m.buildAuthenticationChain("a")
	require.Error(t, err)
}

func TestManager_isCredentialValid(t *testing.T) {
	now := time.Now().UTC()
	s := &testStore{expired: map[string]bool{"ok": false, "expired": true}}
	m := &manager{credentialStore: s}

	// Expired -> false
	valid, exp := m.isCredentialValid("expired", &testCreds{exp: ptrTime(now.Add(1 * time.Hour))})
	assert.False(t, valid)
	assert.Nil(t, exp)

	// Not expired, far future -> true, non-nil exp
	texp := now.Add(10 * time.Minute)
	valid, exp = m.isCredentialValid("ok", &testCreds{exp: &texp})
	assert.True(t, valid)
	require.NotNil(t, exp)

	// Not expired, no expiration -> true, nil exp
	valid, exp = m.isCredentialValid("ok", &testCreds{exp: nil})
	assert.True(t, valid)
	assert.Nil(t, exp)
}

func TestManager_Whoami_Paths(t *testing.T) {
	s := &testStore{data: map[string]any{}, expired: map[string]bool{}}
	m := &manager{
		config: &schema.AuthConfig{Identities: map[string]schema.Identity{
			"dev": {Kind: "aws/user"},
		}},
		identities: map[string]types.Identity{
			"dev": stubIdentity{provider: "p"},
		},
		credentialStore: s,
	}
	// Not found.
	_, err := m.Whoami(context.Background(), "ghost")
	assert.Error(t, err)

	// No creds.
	_, err = m.Whoami(context.Background(), "dev")
	assert.Error(t, err)

	// Expired.
	s.data["dev"] = &testCreds{}
	s.expired["dev"] = true
	_, err = m.Whoami(context.Background(), "dev")
	assert.Error(t, err)

	// Success.
	s.expired["dev"] = false
	info, err := m.Whoami(context.Background(), "dev")
	require.NoError(t, err)
	assert.Equal(t, "p", info.Provider)
	assert.Equal(t, "dev", info.Identity)
	assert.Equal(t, "dev", info.CredentialsRef)
	assert.Nil(t, info.Credentials)
}

func TestManager_authenticateWithProvider_Paths(t *testing.T) {
	s := &testStore{}
	m := &manager{credentialStore: s, providers: map[string]types.Provider{}}
	// Missing provider.
	_, err := m.authenticateWithProvider(context.Background(), "p")
	assert.Error(t, err)

	// Success.
	m.providers["p"] = &testProvider{name: "p", creds: &testCreds{}}
	_, err = m.authenticateWithProvider(context.Background(), "p")
	assert.NoError(t, err)
}

func TestManager_retrieveCachedCredentials_and_determineStart(t *testing.T) {
	s := &testStore{data: map[string]any{"x": &testCreds{}}}
	m := &manager{credentialStore: s, chain: []string{"x"}}
	// Determine start index.
	assert.Equal(t, 0, m.determineStartingIndex(-1))
	assert.Equal(t, 2, m.determineStartingIndex(2))

	// Retrieve credentials succeeds.
	_, err := m.retrieveCachedCredentials(m.chain, 0)
	assert.NoError(t, err)

	// Retrieve credentials returns error.
	s2 := &testStore{retrieveErr: map[string]error{"y": assert.AnError}}
	m2 := &manager{credentialStore: s2, chain: []string{"y"}}
	_, err = m2.retrieveCachedCredentials(m2.chain, 0)
	assert.Error(t, err)
}

func TestManager_initializeProvidersAndIdentities(t *testing.T) {
	// Providers: invalid kind.
	m := &manager{config: &schema.AuthConfig{Providers: map[string]schema.Provider{"bad": {Kind: "unknown"}}}}
	assert.Error(t, m.initializeProviders())

	// Identities: invalid kind.
	m = &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{"x": {Kind: "unknown"}}}}
	assert.Error(t, m.initializeIdentities())
}

func TestManager_GetProviderKindForIdentity_UnknownProvider(t *testing.T) {
	m := &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{
		"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}},
	}}}
	_, err := m.GetProviderKindForIdentity("dev")
	assert.Error(t, err)
}

// ptrTime helper.
func ptrTime(t time.Time) *time.Time { return &t }

func TestManager_findFirstValidCachedCredentials(t *testing.T) {
	s := &testStore{data: map[string]any{}, expired: map[string]bool{}}
	now := time.Now().UTC().Add(10 * time.Minute)
	m := &manager{credentialStore: s, chain: []string{"prov", "id1", "id2"}}

	// Seed: id2 valid, id1 valid.
	s.data["id2"] = &testCreds{exp: &now}
	s.expired["id2"] = false
	s.data["id1"] = &testCreds{exp: &now}
	s.expired["id1"] = false
	idx := m.findFirstValidCachedCredentials()
	require.Equal(t, 2, idx)

	// Mark id2 expired, should pick id1.
	s.expired["id2"] = true
	idx = m.findFirstValidCachedCredentials()
	require.Equal(t, 1, idx)

	// Both expired -> -1.
	s.expired["id1"] = true
	idx = m.findFirstValidCachedCredentials()
	require.Equal(t, -1, idx)
}

func TestManager_authenticateIdentityChain_IdentityMissing(t *testing.T) {
	m := &manager{identities: map[string]types.Identity{}, chain: []string{"prov", "step1"}}
	_, err := m.authenticateIdentityChain(context.Background(), 1, &testCreds{})
	assert.Error(t, err)
}

type stubUserID struct{ out types.ICredentials }

func (s stubUserID) Kind() string                     { return "aws/user" }
func (s stubUserID) GetProviderName() (string, error) { return "aws-user", nil }
func (s stubUserID) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	return s.out, nil
}
func (s stubUserID) Validate() error                         { return nil }
func (s stubUserID) Environment() (map[string]string, error) { return map[string]string{}, nil }
func (s stubUserID) PostAuthenticate(_ context.Context, _ *schema.ConfigAndStacksInfo, _ string, _ string, _ types.ICredentials) error {
	return nil
}

func TestManager_authenticateFromIndex_StandaloneAWSUser(t *testing.T) {
	creds := &testCreds{}
	m := &manager{
		config:     &schema.AuthConfig{Identities: map[string]schema.Identity{"dev": {Kind: "aws/user"}}},
		identities: map[string]types.Identity{"dev": stubUserID{out: creds}},
		chain:      []string{"dev"},
	}
	out, err := m.authenticateFromIndex(context.Background(), -1)
	require.NoError(t, err)
	assert.Equal(t, creds, out)
}

func TestManager_authenticateProviderChain_PreAuthError(t *testing.T) {
	m := &manager{providers: map[string]types.Provider{"p": &testProvider{name: "p", preErr: assert.AnError}}, chain: []string{"p"}}
	_, err := m.authenticateProviderChain(context.Background(), 0)
	assert.Error(t, err)
}

func TestManager_buildWhoamiInfo_SetsRefAndEnv(t *testing.T) {
	s := &testStore{data: map[string]any{}}
	ident := stubIdentity{provider: "p"}
	m := &manager{credentialStore: s, identities: map[string]types.Identity{"dev": ident}}
	exp := time.Now().UTC().Add(30 * time.Minute)
	c := &testCreds{exp: &exp}

	info := m.buildWhoamiInfo("dev", c)
	assert.Equal(t, "p", info.Provider)
	assert.Equal(t, "dev", info.Identity)
	assert.Equal(t, "dev", info.CredentialsRef)
	assert.Nil(t, info.Credentials)
}
