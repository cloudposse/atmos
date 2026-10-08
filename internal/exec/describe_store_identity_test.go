package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	storepkg "github.com/cloudposse/atmos/pkg/store"
)

// recordingStore is an identity-aware store that reports which identity it was bound to.
type recordingStore struct {
	storepkg.IdentityAwareStore
	identity string
	bound    string
	resolver storepkg.AuthContextResolver
}

func (s *recordingStore) IdentityName() string { return s.identity }

func (s *recordingStore) SetAuthContext(resolver storepkg.AuthContextResolver, identityName string) {
	s.resolver, s.bound = resolver, identityName
}

// TestDescribeStoreInheritsExplicitIdentity checks describe binds the caller's identity to
// identity-less stores exactly as deploy does, and leaves everything else alone.
func TestDescribeStoreInheritsExplicitIdentity(t *testing.T) {
	requested := func(value string) *schema.ConfigAndStacksInfo {
		return &schema.ConfigAndStacksInfo{RequestedIdentity: &value}
	}
	for _, tc := range []struct {
		name      string
		info      *schema.ConfigAndStacksInfo
		chain     []string
		storeOwns string
		wantBound string
		wantChain bool
	}{
		{name: "explicit identity", info: requested("sandbox"), wantBound: "sandbox"},
		{name: "explicit identity equal to the default", info: requested("dev"), wantBound: "dev"},
		{name: "environment false value disables", info: requested("false"), wantBound: ""},
		{name: "no request keeps the SDK chain", info: requested(""), wantBound: ""},
		{name: "no requested identity recorded", info: &schema.ConfigAndStacksInfo{}, wantBound: ""},
		{name: "bare prompt uses the chosen identity", info: requested(cfg.IdentityFlagSelectValue), chain: []string{"upstream", "chosen"}, wantBound: "chosen", wantChain: true},
		{name: "store identity wins", info: requested("sandbox"), storeOwns: "store-identity", wantBound: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := authtypes.NewMockAuthManager(gomock.NewController(t))
			manager.EXPECT().GetStackInfo().Return(tc.info)
			if tc.wantChain {
				manager.EXPECT().GetChain().Return(tc.chain)
			}
			store := &recordingStore{identity: tc.storeOwns, bound: "unset"}
			atmosConfig := &schema.AtmosConfiguration{Stores: storepkg.StoreRegistry{"s": store}}

			injectDescribeComponentStoreAuthResolver(atmosConfig, manager)

			require.NotNil(t, store.resolver, "identity-aware stores always receive the resolver")
			assert.Equal(t, tc.wantBound, store.bound)
		})
	}
}

// TestDescribeStacksBindsStoresForExplicitIdentity covers the describe stacks entry point:
// a real authenticated manager binds stores, while the deferred manager is left to bind per read.
func TestDescribeStacksBindsStoresForExplicitIdentity(t *testing.T) {
	ac := setupOutputIdentityFixture(t, "")
	store := &recordingStore{bound: "unset"}
	ac.Stores = storepkg.StoreRegistry{"s": store}

	manager := outputIdentityManager(t, "sandbox")
	_, err := ExecuteDescribeStacks(&ac, "test", []string{"reader"}, nil, nil, false, false, false, false, nil, manager)
	require.NoError(t, err)
	assert.Equal(t, "sandbox", store.bound)
}

// TestPropagateAuthCarriesRequestedIdentity keeps the caller's original selection with the
// authenticated manager, so describe stacks evaluates values under the same request as describe component.
func TestPropagateAuthCarriesRequestedIdentity(t *testing.T) {
	requested := "sandbox"
	manager := authtypes.NewMockAuthManager(gomock.NewController(t))
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{
		RequestedIdentity: &requested,
		AuthContext:       &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "sandbox"}},
	})
	info := &schema.ConfigAndStacksInfo{}

	propagateAuth(info, manager)

	require.NotNil(t, info.RequestedIdentity)
	assert.Equal(t, "sandbox", *info.RequestedIdentity)
	assert.Equal(t, "sandbox", info.AuthContext.AWS.Profile)
	assert.Same(t, manager, info.AuthManager)

	// Without a recorded request, nothing is invented.
	bare := authtypes.NewMockAuthManager(gomock.NewController(t))
	bare.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{})
	untouched := &schema.ConfigAndStacksInfo{}
	propagateAuth(untouched, bare)
	assert.Nil(t, untouched.RequestedIdentity)
}
