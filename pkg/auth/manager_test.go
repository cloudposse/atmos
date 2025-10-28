package auth

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
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

			if tt.isCI {
				t.Setenv("CI", "true")
			} else {
				orig := os.Getenv("CI")
				os.Unsetenv("CI")
				t.Cleanup(func() {
					if orig != "" {
						os.Setenv("CI", orig)
					}
				})
			}

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
	origCI, hadCI := os.LookupEnv("CI")
	t.Setenv("CI", "true")
	defer func() {
		if hadCI {
			os.Setenv("CI", origCI)
		} else {
			os.Unsetenv("CI")
		}
	}()
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
func (c *testCreds) Validate(ctx context.Context) (*types.ValidationInfo, error) {
	return &types.ValidationInfo{Expiration: c.exp}, nil
}

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
func (s *testStore) Type() string { return "test" }

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
func (p *testProvider) Paths() ([]types.Path, error)            { return []types.Path{}, nil }
func (p *testProvider) Logout(_ context.Context) error          { return nil }
func (p *testProvider) GetFilesDisplayPath() string             { return "~/.aws/atmos" }
func (p *testProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}

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
	s := &testStore{}
	m := &manager{credentialStore: s}

	// Expired credentials (expiration in the past) -> false, returns expiration time.
	pastExp := now.Add(-1 * time.Hour)
	valid, exp := m.isCredentialValid("expired", &testCreds{exp: &pastExp})
	assert.False(t, valid)
	assert.NotNil(t, exp) // Returns the expiration time even when invalid.
	assert.Equal(t, pastExp, *exp)

	// Expiring soon (less than 5 minutes) -> false, returns expiration time.
	soonExp := now.Add(3 * time.Minute)
	valid, exp = m.isCredentialValid("expiring-soon", &testCreds{exp: &soonExp})
	assert.False(t, valid)
	assert.NotNil(t, exp)
	assert.Equal(t, soonExp, *exp)

	// Valid credentials with future expiration (>5 minutes) -> true, returns expiration.
	futureExp := now.Add(10 * time.Minute)
	valid, exp = m.isCredentialValid("valid", &testCreds{exp: &futureExp})
	assert.True(t, valid)
	require.NotNil(t, exp)
	assert.Equal(t, futureExp, *exp)

	// Non-expiring credentials (no expiration info) -> true, nil expiration.
	valid, exp = m.isCredentialValid("non-expiring", &testCreds{exp: nil})
	assert.True(t, valid)
	assert.Nil(t, exp)
}

func TestManager_GetCachedCredentials_Paths(t *testing.T) {
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
	_, err := m.GetCachedCredentials(context.Background(), "ghost")
	assert.Error(t, err)

	// No creds.
	_, err = m.GetCachedCredentials(context.Background(), "dev")
	assert.Error(t, err)

	// Expired.
	s.data["dev"] = &testCreds{}
	s.expired["dev"] = true
	_, err = m.GetCachedCredentials(context.Background(), "dev")
	assert.Error(t, err)

	// Success.
	s.expired["dev"] = false
	info, err := m.GetCachedCredentials(context.Background(), "dev")
	require.NoError(t, err)
	assert.Equal(t, "p", info.Provider)
	assert.Equal(t, "dev", info.Identity)
	assert.Equal(t, "dev", info.CredentialsRef)
	assert.NotNil(t, info.Credentials) // Credentials are preserved in WhoamiInfo.
}

func TestManager_Whoami_WithCachedCredentials(t *testing.T) {
	// Test that Whoami successfully retrieves cached credentials when available.
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

	// Setup valid cached credentials.
	s.data["dev"] = &testCreds{}
	s.expired["dev"] = false

	// Whoami should use GetCachedCredentials and succeed.
	info, err := m.Whoami(context.Background(), "dev")
	require.NoError(t, err)
	assert.Equal(t, "p", info.Provider)
	assert.Equal(t, "dev", info.Identity)
	assert.Equal(t, "dev", info.CredentialsRef)
	assert.NotNil(t, info.Credentials) // Credentials are preserved in WhoamiInfo.
}

func TestManager_Whoami_FallbackAuthenticationFails(t *testing.T) {
	// Test that Whoami returns error when both GetCachedCredentials and Authenticate fail.
	// This covers the case where no cached credentials exist and reauthentication also fails.
	s := &testStore{data: map[string]any{}, expired: map[string]bool{}}
	m := &manager{
		config: &schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"p": {Kind: "test"},
			},
			Identities: map[string]schema.Identity{
				"dev": {Kind: "test", Via: &schema.IdentityVia{Provider: "p"}},
			},
		},
		identities: map[string]types.Identity{
			"dev": stubPSIdentity{provider: "p"},
		},
		providers: map[string]types.Provider{
			// Provider that fails authentication.
			"p": &testProvider{name: "p", authErr: fmt.Errorf("provider auth failed")},
		},
		credentialStore: s,
		validator:       dummyValidator{},
	}

	// No cached credentials exist (empty store).
	// Whoami should try GetCachedCredentials (fail), then fall back to Authenticate (also fail).
	info, err := m.Whoami(context.Background(), "dev")

	// Should return error.
	// Note: Returns the original GetCachedCredentials error, not the Authenticate error.
	assert.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "no credentials found")
}

func TestManager_Whoami_FallbackAuthenticationSucceeds(t *testing.T) {
	// Test that Whoami succeeds via fallback authentication when no cached credentials exist.
	// This covers the case where provider credentials exist (e.g., in AWS files) and can be used
	// to derive identity credentials without interactive prompts.
	s := &testStore{data: map[string]any{}, expired: map[string]bool{}}
	m := &manager{
		config: &schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"p": {Kind: "test"},
			},
			Identities: map[string]schema.Identity{
				"dev": {Kind: "test", Via: &schema.IdentityVia{Provider: "p"}},
			},
		},
		identities: map[string]types.Identity{
			"dev": stubPSIdentity{provider: "p", out: &testCreds{}},
		},
		providers: map[string]types.Provider{
			// Provider that succeeds authentication.
			"p": &testProvider{name: "p", creds: &testCreds{}},
		},
		credentialStore: s,
		validator:       dummyValidator{},
	}

	// No cached credentials exist (empty store).
	// Whoami should try GetCachedCredentials (fail), then fall back to Authenticate (succeed).
	info, err := m.Whoami(context.Background(), "dev")

	// Should succeed via fallback authentication.
	require.NoError(t, err)
	assert.Equal(t, "p", info.Provider)
	assert.Equal(t, "dev", info.Identity)
	assert.NotNil(t, info.Credentials)
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
	s := &testStore{data: map[string]any{}}
	now := time.Now().UTC()
	validExp := now.Add(10 * time.Minute) // Valid: expires in 10 minutes.
	expiredExp := now.Add(-1 * time.Hour) // Expired: expired 1 hour ago.
	m := &manager{credentialStore: s, chain: []string{"prov", "id1", "id2"}}

	// Both id1 and id2 have valid credentials -> should return id2 (last in chain).
	s.data["id2"] = &testCreds{exp: &validExp}
	s.data["id1"] = &testCreds{exp: &validExp}
	idx := m.findFirstValidCachedCredentials()
	require.Equal(t, 2, idx)

	// id2 expired, id1 still valid -> should pick id1.
	s.data["id2"] = &testCreds{exp: &expiredExp}
	idx = m.findFirstValidCachedCredentials()
	require.Equal(t, 1, idx)

	// Both expired -> should return -1 (no valid credentials).
	s.data["id1"] = &testCreds{exp: &expiredExp}
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
func (s stubUserID) Paths() ([]types.Path, error)            { return []types.Path{}, nil }
func (s stubUserID) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}
func (s stubUserID) Logout(_ context.Context) error                                { return nil }
func (s stubUserID) CredentialsExist() (bool, error)                               { return true, nil }
func (s stubUserID) LoadCredentials(_ context.Context) (types.ICredentials, error) { return nil, nil }
func (s stubUserID) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
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
	assert.NotNil(t, info.Credentials) // Credentials are preserved for validation purposes.
}

// dummyValidator implements types.Validator for tests.
type dummyValidator struct{}

func (dummyValidator) ValidateAuthConfig(_ *schema.AuthConfig) error       { return nil }
func (dummyValidator) ValidateProvider(_ string, _ *schema.Provider) error { return nil }
func (dummyValidator) ValidateIdentity(_ string, _ *schema.Identity, _ map[string]*schema.Provider) error {
	return nil
}

func (dummyValidator) ValidateChains(_ map[string]*schema.Identity, _ map[string]*schema.Provider) error {
	return nil
}

// stubPSIdentity is a simple permission-set identity for Authenticate() tests.
type stubPSIdentity struct {
	provider   string
	out        types.ICredentials
	postCalled *bool
	postErr    error
	env        map[string]string
}

func (s stubPSIdentity) Kind() string                     { return "aws/permission-set" }
func (s stubPSIdentity) GetProviderName() (string, error) { return s.provider, nil }
func (s stubPSIdentity) Authenticate(_ context.Context, base types.ICredentials) (types.ICredentials, error) {
	if s.out != nil {
		return s.out, nil
	}
	return base, nil
}
func (s stubPSIdentity) Validate() error                         { return nil }
func (s stubPSIdentity) Environment() (map[string]string, error) { return s.env, nil }
func (s stubPSIdentity) Paths() ([]types.Path, error)            { return []types.Path{}, nil }
func (s stubPSIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	if s.postCalled != nil {
		*s.postCalled = true
	}
	return s.postErr
}
func (s stubPSIdentity) Logout(_ context.Context) error  { return nil }
func (s stubPSIdentity) CredentialsExist() (bool, error) { return true, nil }
func (s stubPSIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, nil
}

func (s stubPSIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}

func TestNewAuthManager_ParamValidation(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := NewAuthManager(nil, &testStore{}, dummyValidator{}, nil)
		assert.ErrorIs(t, err, errUtils.ErrNilParam)
	})
	t.Run("nil store", func(t *testing.T) {
		_, err := NewAuthManager(&schema.AuthConfig{}, nil, dummyValidator{}, nil)
		assert.ErrorIs(t, err, errUtils.ErrNilParam)
	})
	t.Run("nil validator", func(t *testing.T) {
		_, err := NewAuthManager(&schema.AuthConfig{}, &testStore{}, nil, nil)
		assert.ErrorIs(t, err, errUtils.ErrNilParam)
	})
}

func TestNewAuthManager_InitializeErrors(t *testing.T) {
	t.Run("invalid provider kind", func(t *testing.T) {
		cfg := &schema.AuthConfig{Providers: map[string]schema.Provider{"bad": {Kind: "unknown"}}}
		_, err := NewAuthManager(cfg, &testStore{}, dummyValidator{}, nil)
		assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
	})

	t.Run("invalid identity kind", func(t *testing.T) {
		cfg := &schema.AuthConfig{Identities: map[string]schema.Identity{"x": {Kind: "unknown"}}}
		_, err := NewAuthManager(cfg, &testStore{}, dummyValidator{}, nil)
		assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
	})
}

func TestManager_Authenticate_Errors(t *testing.T) {
	m := &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{}}}

	// Empty identity name.
	_, err := m.Authenticate(context.Background(), "")
	assert.ErrorIs(t, err, errUtils.ErrNilParam)

	// Identity not found.
	_, err = m.Authenticate(context.Background(), "missing")
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)

	// Identity present but invalid chain.
	m = &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{
		"dev": {Kind: "aws/permission-set"},
	}}}
	_, err = m.Authenticate(context.Background(), "dev")
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
}

func TestManager_Authenticate_SuccessFlow(t *testing.T) {
	s := &testStore{data: map[string]any{}, expired: map[string]bool{}}
	called := false

	// Build a provider-based chain: p -> dev.
	m := &manager{
		config: &schema.AuthConfig{
			Providers: map[string]schema.Provider{"p": {Kind: "aws/iam-identity-center"}},
			Identities: map[string]schema.Identity{
				"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}},
			},
		},
		providers:       map[string]types.Provider{"p": &testProvider{name: "p", creds: &testCreds{}}},
		identities:      map[string]types.Identity{"dev": stubPSIdentity{provider: "p", out: &testCreds{}, postCalled: &called, env: map[string]string{"FOO": "BAR"}}},
		credentialStore: s,
		validator:       dummyValidator{},
	}

	info, err := m.Authenticate(context.Background(), "dev")
	require.NoError(t, err)
	assert.Equal(t, []string{"p", "dev"}, m.GetChain())
	assert.Equal(t, "p", info.Provider)
	assert.Equal(t, "dev", info.Identity)
	assert.Equal(t, "dev", info.CredentialsRef)
	assert.NotNil(t, info.Credentials) // Credentials are preserved in WhoamiInfo.
	assert.Equal(t, "BAR", info.Environment["FOO"])
	assert.True(t, called, "PostAuthenticate should be called")
}

func TestManager_Authenticate_UsesCachedTargetCredentials(t *testing.T) {
	now := ptrTime(time.Now().UTC().Add(30 * time.Minute))

	// Pre-seed store with valid creds for target identity.
	s := &testStore{data: map[string]any{"dev": &testCreds{exp: now}}, expired: map[string]bool{"dev": false}}

	called := false
	m := &manager{
		config: &schema.AuthConfig{
			Providers: map[string]schema.Provider{"p": {Kind: "aws/iam-identity-center"}},
			Identities: map[string]schema.Identity{
				"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}},
			},
		},
		providers:       map[string]types.Provider{"p": &testProvider{name: "p", creds: &testCreds{}}},
		identities:      map[string]types.Identity{"dev": stubPSIdentity{provider: "p", out: &testCreds{}, postCalled: &called}},
		credentialStore: s,
		validator:       dummyValidator{},
	}

	info, err := m.Authenticate(context.Background(), "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev", info.Identity)
	assert.True(t, called, "PostAuthenticate should be called when using cached credentials")
}

func TestManager_Authenticate_ExpiredCredentials(t *testing.T) {
	// Create expired credentials.
	expiredTime := ptrTime(time.Now().UTC().Add(-time.Hour))

	// Pre-seed store with expired creds for target identity.
	s := &testStore{
		data:    map[string]any{"dev": &testCreds{exp: expiredTime}},
		expired: map[string]bool{"dev": true}, // Mark as expired
	}

	called := false
	m := &manager{
		config: &schema.AuthConfig{
			Providers: map[string]schema.Provider{"p": {Kind: "aws/iam-identity-center"}},
			Identities: map[string]schema.Identity{
				"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}},
			},
		},
		providers:       map[string]types.Provider{"p": &testProvider{name: "p", creds: &testCreds{}}},
		identities:      map[string]types.Identity{"dev": stubPSIdentity{provider: "p", out: &testCreds{}, postCalled: &called}},
		credentialStore: s,
		validator:       dummyValidator{},
	}

	// Should perform fresh authentication since cached credentials are expired.
	info, err := m.Authenticate(context.Background(), "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev", info.Identity)
	assert.True(t, called, "PostAuthenticate should be called for fresh authentication")
}

func TestManager_ListProviders(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"sso":    {Kind: "aws/iam-identity-center"},
				"saml":   {Kind: "aws/saml"},
				"github": {Kind: "github/oidc"},
			},
		},
		providers: map[string]types.Provider{
			"sso":    &testProvider{name: "sso"},
			"saml":   &testProvider{name: "saml"},
			"github": &testProvider{name: "github"},
		},
	}

	providers := m.ListProviders()
	assert.Len(t, providers, 3)
	assert.Contains(t, providers, "sso")
	assert.Contains(t, providers, "saml")
	assert.Contains(t, providers, "github")
}

func TestManager_GetProviders(t *testing.T) {
	expectedProviders := map[string]schema.Provider{
		"sso":  {Kind: "aws/iam-identity-center", Region: "us-east-1"},
		"saml": {Kind: "aws/saml", URL: "https://example.com", Region: "us-west-2"},
	}

	m := &manager{
		config: &schema.AuthConfig{
			Providers: expectedProviders,
		},
	}

	providers := m.GetProviders()
	assert.Equal(t, expectedProviders, providers)
	assert.Equal(t, "aws/iam-identity-center", providers["sso"].Kind)
	assert.Equal(t, "aws/saml", providers["saml"].Kind)
	assert.Equal(t, "us-east-1", providers["sso"].Region)
	assert.Equal(t, "us-west-2", providers["saml"].Region)
}

func TestManager_GetIdentities(t *testing.T) {
	expectedIdentities := map[string]schema.Identity{
		"dev":  {Kind: "aws/user", Default: true},
		"prod": {Kind: "aws/assume-role", Principal: map[string]any{"assume_role": "arn:aws:iam::123:role/Prod"}},
	}

	m := &manager{
		config: &schema.AuthConfig{
			Identities: expectedIdentities,
		},
	}

	identities := m.GetIdentities()
	assert.Equal(t, expectedIdentities, identities)
	assert.Equal(t, "aws/user", identities["dev"].Kind)
	assert.Equal(t, "aws/assume-role", identities["prod"].Kind)
	assert.True(t, identities["dev"].Default)
	assert.False(t, identities["prod"].Default)
}

func TestManager_GetStackInfo(t *testing.T) {
	stackInfo := &schema.ConfigAndStacksInfo{
		ComponentEnvSection: schema.AtmosSectionMapType{"TEST": "value"},
		Identity:            "test-identity",
	}

	m := &manager{
		stackInfo: stackInfo,
	}

	result := m.GetStackInfo()
	assert.Equal(t, stackInfo, result)
	assert.Equal(t, "value", result.ComponentEnvSection["TEST"])
	assert.Equal(t, "test-identity", result.Identity)
}

func TestManager_GetChain_Empty(t *testing.T) {
	m := &manager{}

	chain := m.GetChain()
	assert.Empty(t, chain)
}

func TestManager_GetChain_WithData(t *testing.T) {
	m := &manager{
		chain: []string{"provider", "identity1", "identity2"},
	}

	chain := m.GetChain()
	assert.Equal(t, []string{"provider", "identity1", "identity2"}, chain)
}

// stubIdentity implements types.Identity minimally for provider lookups.
type stubIdentity struct{ provider string }

func (s stubIdentity) Kind() string                     { return "aws/permission-set" }
func (s stubIdentity) GetProviderName() (string, error) { return s.provider, nil }
func (s stubIdentity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	return nil, nil
}
func (s stubIdentity) Validate() error                         { return nil }
func (s stubIdentity) Environment() (map[string]string, error) { return nil, nil }
func (s stubIdentity) Paths() ([]types.Path, error)            { return []types.Path{}, nil }
func (s stubIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}
func (s stubIdentity) Logout(_ context.Context) error                                { return nil }
func (s stubIdentity) CredentialsExist() (bool, error)                               { return true, nil }
func (s stubIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) { return nil, nil }
func (s stubIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}

func TestBuildAuthenticationChain_Basic(t *testing.T) {
	m := &manager{config: &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"aws-sso": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "aws-sso"}},
		},
	}}

	chain, err := m.buildAuthenticationChain("dev")
	assert.NoError(t, err)
	assert.Equal(t, []string{"aws-sso", "dev"}, chain)
}

func TestBuildAuthenticationChain_NestedIdentity(t *testing.T) {
	t.Parallel()

	m := &manager{config: &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"p": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"child":  {Kind: "aws/permission-set", Via: &schema.IdentityVia{Identity: "root"}},
			"root":   {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}},
			"orphan": {Kind: "aws/user"},
		},
	}}

	chain, err := m.buildAuthenticationChain("child")
	require.NoError(t, err)
	assert.Equal(t, []string{"p", "root", "child"}, chain)

	// aws/user without via produces identity-only chain.
	only, err := m.buildAuthenticationChain("orphan")
	require.NoError(t, err)
	assert.Equal(t, []string{"orphan"}, only)
}

func TestGetProviderKindForIdentity(t *testing.T) {
	t.Parallel()

	m := &manager{config: &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"p": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "p"}},
			"me":  {Kind: "aws/user"},
		},
	}}
	// Populate identities map so alias resolution can use GetProviderName().
	m.identities = map[string]types.Identity{"alias": stubIdentity{provider: "p"}}

	kind, err := m.GetProviderKindForIdentity("dev")
	require.NoError(t, err)
	assert.Equal(t, "aws/iam-identity-center", kind)

	// For aws/user chain root is the identity itself.
	kind, err = m.GetProviderKindForIdentity("me")
	require.NoError(t, err)
	assert.Equal(t, "aws/user", kind)

	_, err = m.GetProviderKindForIdentity("developer")
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}

func TestManager_GetConfig(t *testing.T) {
	stackInfo := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{"test": "value"},
	}

	m := &manager{
		stackInfo: stackInfo,
	}

	config := m.GetConfig()
	assert.Equal(t, stackInfo, config)
}

func TestManager_GetProviderForIdentity(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"sso": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"admin": {
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{Provider: "sso"},
			},
			"me": {
				Kind: "aws/user",
			},
		},
	}

	m := &manager{
		config:          authConfig,
		providers:       make(map[string]types.Provider),
		identities:      make(map[string]types.Identity),
		credentialStore: &testStore{},
	}

	// Test with permission set identity.
	provider := m.GetProviderForIdentity("admin")
	assert.Equal(t, "sso", provider)

	// Test with aws/user identity (standalone).
	provider = m.GetProviderForIdentity("me")
	assert.Equal(t, "aws-user", provider)

	// Test with non-existent identity.
	provider = m.GetProviderForIdentity("nonexistent")
	assert.Equal(t, "", provider)
}

func TestManager_Validate(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"sso": {Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://example.awsapps.com/start"},
		},
		Identities: map[string]schema.Identity{
			"admin": {
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{Provider: "sso"},
				Principal: map[string]any{
					"name": "admin",
					"account": map[string]any{
						"id": "123456789012",
					},
				},
			},
		},
	}

	m := &manager{
		config:          authConfig,
		providers:       make(map[string]types.Provider),
		identities:      make(map[string]types.Identity),
		credentialStore: &testStore{},
		validator:       &testValidator{},
	}

	err := m.Validate()
	assert.NoError(t, err)
}

type testValidator struct {
	validateErr error
}

func (v *testValidator) ValidateAuthConfig(config *schema.AuthConfig) error {
	return v.validateErr
}
func (v *testValidator) ValidateLogsConfig(logs *schema.Logs) error { return nil }
func (v *testValidator) ValidateProvider(name string, provider *schema.Provider) error {
	return nil
}

func (v *testValidator) ValidateIdentity(name string, identity *schema.Identity, providers map[string]*schema.Provider) error {
	return nil
}

func (v *testValidator) ValidateChains(identities map[string]*schema.Identity, providers map[string]*schema.Provider) error {
	return nil
}

func TestManager_fetchCachedCredentials(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	creds := &testCreds{exp: &future}

	m := &manager{
		chain: []string{"sso", "admin"},
		credentialStore: &testStore{
			data: map[string]any{
				"admin": creds,
			},
		},
	}

	// Test successful retrieval.
	retrievedCreds, nextIndex := m.fetchCachedCredentials(1)
	assert.NotNil(t, retrievedCreds)
	assert.Equal(t, 2, nextIndex)

	// Test failed retrieval - should return nil and 0.
	m2 := &manager{
		chain: []string{"sso", "admin"},
		credentialStore: &testStore{
			retrieveErr: map[string]error{
				"admin": errUtils.ErrNoCredentialsFound,
			},
		},
	}
	retrievedCreds, nextIndex = m2.fetchCachedCredentials(1)
	assert.Nil(t, retrievedCreds)
	assert.Equal(t, 0, nextIndex)
}

func TestManager_GetFilesDisplayPath(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		provider     types.Provider
		expected     string
	}{
		{
			name:         "provider exists",
			providerName: "test-provider",
			provider:     &testProvider{name: "test-provider"},
			expected:     "~/.aws/atmos",
		},
		{
			name:         "provider not found",
			providerName: "non-existent",
			provider:     nil,
			expected:     "~/.aws/atmos", // Default fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{
				providers: make(map[string]types.Provider),
			}

			if tt.provider != nil {
				m.providers[tt.providerName] = tt.provider
			}

			path := m.GetFilesDisplayPath(tt.providerName)
			assert.Equal(t, tt.expected, path)
		})
	}
}

// stubEnvIdentity is a test stub for testing GetEnvironmentVariables and buildWhoamiInfoFromEnvironment.
type stubEnvIdentity struct {
	provider         string
	env              map[string]string
	envErr           error
	loadCreds        types.ICredentials
	loadCredsErr     error
	credentialsExist bool
}

func (s *stubEnvIdentity) Kind() string { return "test" }
func (s *stubEnvIdentity) GetProviderName() (string, error) {
	if s.provider == "" {
		return "test-provider", nil
	}
	return s.provider, nil
}

func (s *stubEnvIdentity) Authenticate(_ context.Context, base types.ICredentials) (types.ICredentials, error) {
	return base, nil
}
func (s *stubEnvIdentity) Validate() error { return nil }
func (s *stubEnvIdentity) Environment() (map[string]string, error) {
	return s.env, s.envErr
}

func (s *stubEnvIdentity) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

func (s *stubEnvIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}
func (s *stubEnvIdentity) Logout(_ context.Context) error { return nil }
func (s *stubEnvIdentity) CredentialsExist() (bool, error) {
	return s.credentialsExist, nil
}

func (s *stubEnvIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return s.loadCreds, s.loadCredsErr
}

func (s *stubEnvIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}

func TestManager_GetEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		identity     types.Identity
		expectError  bool
		expectedVars map[string]string
	}{
		{
			name:         "identity exists with environment variables",
			identityName: "test-identity",
			identity: &stubEnvIdentity{
				env: map[string]string{
					"AWS_PROFILE":     "test-profile",
					"AWS_CONFIG_FILE": "/path/to/config",
				},
			},
			expectError: false,
			expectedVars: map[string]string{
				"AWS_PROFILE":     "test-profile",
				"AWS_CONFIG_FILE": "/path/to/config",
			},
		},
		{
			name:         "identity not found",
			identityName: "nonexistent",
			identity:     nil,
			expectError:  true,
		},
		{
			name:         "identity with empty environment",
			identityName: "empty-identity",
			identity: &stubEnvIdentity{
				env: map[string]string{},
			},
			expectError:  false,
			expectedVars: map[string]string{},
		},
		{
			name:         "identity environment returns error",
			identityName: "error-identity",
			identity: &stubEnvIdentity{
				envErr: fmt.Errorf("environment generation failed"),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{
				identities: make(map[string]types.Identity),
			}

			if tt.identity != nil {
				m.identities[tt.identityName] = tt.identity
			}

			vars, err := m.GetEnvironmentVariables(tt.identityName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, vars)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVars, vars)
			}
		})
	}
}

func TestManager_buildWhoamiInfoFromEnvironment(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		identity     types.Identity
		providerName string
		expectEnv    map[string]string
		expectCreds  bool
	}{
		{
			name:         "identity with environment and credentials",
			identityName: "test-identity",
			providerName: "test-provider",
			identity: &stubEnvIdentity{
				provider: "test-provider",
				env: map[string]string{
					"AWS_PROFILE":     "test-profile",
					"AWS_CONFIG_FILE": "/path/to/config",
				},
				loadCreds: &testCreds{},
			},
			expectEnv: map[string]string{
				"AWS_PROFILE":     "test-profile",
				"AWS_CONFIG_FILE": "/path/to/config",
			},
			expectCreds: true,
		},
		{
			name:         "identity with environment but no credentials",
			identityName: "env-only",
			providerName: "test-provider",
			identity: &stubEnvIdentity{
				provider: "test-provider",
				env: map[string]string{
					"FOO": "bar",
				},
				loadCreds: nil,
			},
			expectEnv: map[string]string{
				"FOO": "bar",
			},
			expectCreds: false,
		},
		{
			name:         "identity with credentials load error",
			identityName: "creds-error",
			providerName: "test-provider",
			identity: &stubEnvIdentity{
				provider: "test-provider",
				env: map[string]string{
					"FOO": "bar",
				},
				loadCredsErr: fmt.Errorf("failed to load credentials"),
			},
			expectEnv: map[string]string{
				"FOO": "bar",
			},
			expectCreds: false,
		},
		{
			name:         "identity with environment error",
			identityName: "env-error",
			providerName: "test-provider",
			identity: &stubEnvIdentity{
				provider:  "test-provider",
				envErr:    fmt.Errorf("environment error"),
				loadCreds: &testCreds{},
			},
			expectEnv:   nil,
			expectCreds: true,
		},
		{
			name:         "identity not found",
			identityName: "nonexistent",
			providerName: "",
			identity:     nil,
			expectEnv:    nil,
			expectCreds:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{
				identities: make(map[string]types.Identity),
				providers:  make(map[string]types.Provider),
			}

			if tt.identity != nil {
				m.identities[tt.identityName] = tt.identity
				if tt.providerName != "" {
					m.providers[tt.providerName] = &testProvider{name: tt.providerName}
				}
			}

			info := m.buildWhoamiInfoFromEnvironment(tt.identityName)

			assert.NotNil(t, info)
			assert.Equal(t, tt.identityName, info.Identity)
			assert.Equal(t, tt.providerName, info.Provider)

			if tt.expectEnv != nil {
				assert.Equal(t, tt.expectEnv, info.Environment)
			}

			if tt.expectCreds {
				assert.NotNil(t, info.Credentials)
			} else {
				assert.Nil(t, info.Credentials)
			}
		})
	}
}
