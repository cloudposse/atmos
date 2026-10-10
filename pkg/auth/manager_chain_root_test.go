package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Combine generated mocks for the identity's required and optional interfaces.
type mockStandaloneChainIdentity struct {
	*types.MockIdentity
	*types.MockStandaloneIdentity
}

func newStandaloneChainManager(t *testing.T, roleCount int) (*manager, *mockStandaloneChainIdentity, *types.MockCredentialStore) {
	t.Helper()
	// Missing-credential diagnostics must not inspect the developer's auth files.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctrl := gomock.NewController(t)
	root := &mockStandaloneChainIdentity{
		MockIdentity:           types.NewMockIdentity(ctrl),
		MockStandaloneIdentity: types.NewMockStandaloneIdentity(ctrl),
	}
	store := types.NewMockCredentialStore(ctrl)
	m := &manager{
		config: &schema.AuthConfig{Identities: map[string]schema.Identity{
			"user": {Kind: types.IdentityKindAWSUser},
		}},
		identities:      map[string]types.Identity{"user": root},
		credentialStore: store,
	}
	target := "user"
	for n := 1; n <= roleCount; n++ {
		name := fmt.Sprintf("role%d", n)
		m.config.Identities[name] = schema.Identity{
			Kind: "aws/assume-role", Via: &schema.IdentityVia{Identity: target},
		}
		role := types.NewMockIdentity(ctrl)
		role.EXPECT().Kind().Return("aws/assume-role").AnyTimes()
		m.identities[name] = role
		target = name
	}
	chain, err := m.buildAuthenticationChain(target)
	require.NoError(t, err)
	m.chain = chain
	return m, root, store
}

func chainSession(token string, expiration time.Time) *types.AWSCredentials {
	return &types.AWSCredentials{SessionToken: token, Expiration: expiration.Format(time.RFC3339)}
}

func TestManager_StandaloneRootChain(t *testing.T) {
	future := time.Now().Add(time.Hour)
	session := chainSession("user-session", future)
	longLived := &types.AWSCredentials{AccessKeyID: "test-user-key", SecretAccessKey: "test-secret"}
	keyringMFA := *longLived
	keyringMFA.MfaArn = "arn:aws:iam::111111111111:mfa/test-user"
	tests := []struct {
		name         string
		roles        int
		keyring      types.ICredentials
		file         types.ICredentials
		yamlMFA      bool
		reuseUser    bool
		intermediate bool
		rootError    error
	}{
		{name: "missing session", roles: 1},
		{name: "expired session", roles: 1, file: chainSession("expired", time.Now().Add(-time.Hour))},
		{name: "two downstream roles", roles: 2},
		{name: "keyring keys and YAML MFA", roles: 1, keyring: longLived, yamlMFA: true},
		{name: "keyring keys and stored MFA", roles: 1, keyring: &keyringMFA},
		{name: "keyring keys without MFA", roles: 1, keyring: longLived},
		{name: "keyring keys and expired file session", roles: 1, keyring: longLived, file: chainSession("expired", time.Now().Add(-time.Hour))},
		{name: "valid file session", roles: 1, file: session, reuseUser: true},
		{name: "valid file session with keyring keys", roles: 1, keyring: longLived, file: session, reuseUser: true},
		{name: "valid keyring session", roles: 1, keyring: session, reuseUser: true},
		{name: "valid intermediate session", roles: 2, intermediate: true},
		{name: "root authentication failure", roles: 1, rootError: assert.AnError},
		{name: "direct standalone login", roles: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetProcessCredentialCache()
			t.Cleanup(resetProcessCredentialCache)
			m, root, store := newStandaloneChainManager(t, tt.roles)
			if tt.yamlMFA {
				user := m.config.Identities["user"]
				user.Credentials = map[string]any{"mfa_arn": keyringMFA.MfaArn}
				m.config.Identities["user"] = user
			}
			expectedChain := []string{"user"}
			for n := 1; n <= tt.roles; n++ {
				expectedChain = append(expectedChain, fmt.Sprintf("role%d", n))
			}
			require.Equal(t, expectedChain, m.chain)

			var calls []any
			var input types.ICredentials = session
			if tt.intermediate {
				input = chainSession("cached-intermediate", future)
			} else {
				keyringErr := error(nil)
				if tt.keyring == nil {
					keyringErr = credentials.ErrCredentialsNotFound
				}
				store.EXPECT().Retrieve("user", "").Return(tt.keyring, keyringErr).AnyTimes()
				root.MockIdentity.EXPECT().LoadCredentials(gomock.Any()).Return(tt.file, nil).AnyTimes()
				if !tt.reuseUser {
					root.MockStandaloneIdentity.EXPECT().IsStandalone().Return(true)
					calls = append(calls, root.MockStandaloneIdentity.EXPECT().AuthenticateStandalone(gomock.Any()).Return(session, tt.rootError))
				}
			}
			for n := 1; n <= tt.roles; n++ {
				name := fmt.Sprintf("role%d", n)
				role := m.identities[name].(*types.MockIdentity)
				if tt.intermediate && n == 1 {
					store.EXPECT().Retrieve(name, "").Return(input, nil).Times(2)
					continue
				}
				store.EXPECT().Retrieve(name, "").Return(nil, credentials.ErrCredentialsNotFound)
				role.EXPECT().LoadCredentials(gomock.Any()).Return(nil, nil)
				if tt.rootError == nil {
					output := chainSession(name+"-session", future)
					calls = append(calls, role.EXPECT().Authenticate(gomock.Any(), gomock.Eq(input)).Return(output, nil))
					input = output
				}
			}
			gomock.InOrder(calls...)
			result, err := m.authenticateChain(context.Background(), m.chain[len(m.chain)-1])
			if tt.rootError != nil {
				require.ErrorIs(t, err, tt.rootError)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			assert.Same(t, input, result)
			// No Store expectation: neither user keys nor session credentials may be
			// overwritten in the keyring by the chain manager.
		})
	}
}

func TestManager_ChainCredentialEligibility(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name  string
		kind  string
		creds types.ICredentials
		valid bool
	}{
		{name: "user keys", creds: &types.AWSCredentials{}},
		{name: "user keys with expiration", creds: &types.AWSCredentials{Expiration: future.Format(time.RFC3339)}},
		{name: "session without expiration", creds: &types.AWSCredentials{SessionToken: "token"}},
		{name: "malformed expiration", creds: &types.AWSCredentials{SessionToken: "token", Expiration: "invalid"}},
		{name: "expired session", creds: chainSession("token", time.Now().Add(-time.Hour))},
		{name: "session inside safety buffer", creds: chainSession("token", time.Now().Add(minCredentialValidityBuffer/2))},
		{name: "valid session", creds: chainSession("token", future), valid: true},
		{name: "nil credentials"},
		{name: "typed nil credentials", creds: (*types.AWSCredentials)(nil)},
		{name: "wrong credential type", creds: &testCreds{}},
		{name: "other non-expiring credentials", kind: "other", creds: &testCreds{}, valid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind := tt.kind
			if kind == "" {
				kind = types.IdentityKindAWSUser
			}
			m := &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{
				"root": {Kind: kind},
			}}}
			valid, _ := m.isChainCredentialValid("root", tt.creds)
			assert.Equal(t, tt.valid, valid)
		})
	}
}

func TestManager_StandaloneRootCacheChanges(t *testing.T) {
	future := time.Now().Add(time.Hour)
	session := chainSession("cached-user", future)
	tests := []struct {
		name   string
		creds  types.ICredentials
		err    error
		reauth bool
	}{
		{name: "unchanged session", creds: session},
		{name: "falls back to long-lived keys", creds: &types.AWSCredentials{}, reauth: true},
		{name: "session expires", creds: chainSession("expired", time.Now().Add(-time.Hour)), reauth: true},
		{name: "session loses expiration", creds: &types.AWSCredentials{SessionToken: "token"}, reauth: true},
		{name: "cache retrieval fails", err: assert.AnError, reauth: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, root, store := newStandaloneChainManager(t, 1)
			role := m.identities["role1"].(*types.MockIdentity)
			store.EXPECT().Retrieve("role1", "").Return(nil, credentials.ErrCredentialsNotFound)
			role.EXPECT().LoadCredentials(gomock.Any()).Return(nil, nil)
			gomock.InOrder(
				store.EXPECT().Retrieve("user", "").Return(session, nil),
				store.EXPECT().Retrieve("user", "").Return(tt.creds, tt.err),
			)
			root.MockIdentity.EXPECT().LoadCredentials(gomock.Any()).Return(nil, nil).AnyTimes()
			input := session
			if tt.reauth {
				input = chainSession("new-user-session", future)
				root.MockStandaloneIdentity.EXPECT().IsStandalone().Return(true)
				root.MockStandaloneIdentity.EXPECT().AuthenticateStandalone(gomock.Any()).Return(input, nil)
			}
			output := chainSession("role-session", future)
			role.EXPECT().Authenticate(gomock.Any(), gomock.Eq(input)).Return(output, nil)
			index := m.findFirstValidCachedCredentials()
			require.Equal(t, 0, index)
			result, err := m.authenticateFromIndex(context.Background(), index)
			require.NoError(t, err)
			assert.Same(t, output, result)
		})
	}
}
