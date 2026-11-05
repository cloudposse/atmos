package ci

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// stubAuthManager satisfies authtypes.AuthManager for type-assertion checks.
// The embedded interface is intentional — these tests only exercise the
// type assertion in attachIdentity, never the manager's methods. The
// compile-time assertion below ensures future interface renames or method
// additions fail the build rather than silently pass via embedding.
type stubAuthManager struct {
	authtypes.AuthManager
}

var _ authtypes.AuthManager = (*stubAuthManager)(nil)

// stubResolver is a sentinel AuthContextResolver returned by the test factory.
type stubResolver struct {
	store.AuthContextResolver
}

func (stubResolver) ResolveAWSAuthContext(_ context.Context, _ string) (*store.AWSAuthConfig, error) {
	return nil, nil
}

func (stubResolver) ResolveAzureAuthContext(_ context.Context, _ string) (*store.AzureAuthConfig, error) {
	return nil, nil
}

func (stubResolver) ResolveGCPAuthContext(_ context.Context, _ string) (*store.GCPAuthConfig, error) {
	return nil, nil
}

// swapResolverFactory replaces defaultResolverFactory for the test and
// restores the original via t.Cleanup, removing the manual save/defer
// boilerplate from each test.
func swapResolverFactory(t *testing.T, replacement func(authtypes.AuthManager, *schema.ConfigAndStacksInfo) store.AuthContextResolver) {
	t.Helper()
	original := defaultResolverFactory
	defaultResolverFactory = replacement
	t.Cleanup(func() { defaultResolverFactory = original })
}

// TestAttachIdentity_PropagatesActiveCommandIdentity covers the issue #2369
// scenario: --identity / ATMOS_IDENTITY on the parent command must reach the
// planfile store so the S3 upload authenticates against the same identity.
func TestAttachIdentity_PropagatesActiveCommandIdentity(t *testing.T) {
	called := 0
	swapResolverFactory(t, func(_ authtypes.AuthManager, _ *schema.ConfigAndStacksInfo) store.AuthContextResolver {
		called++
		return stubResolver{}
	})

	info := &schema.ConfigAndStacksInfo{
		Identity:    "ci-deployer",
		AuthManager: &stubAuthManager{},
	}
	artOpts := artifact.StoreOptions{}
	attachIdentity(&artOpts, info)

	assert.Equal(t, "ci-deployer", artOpts.Identity)
	assert.NotNil(t, artOpts.Resolver)
	assert.Equal(t, 1, called)
}

// TestAttachIdentity_NoIdentityNoResolver verifies nothing is attached when
// no identity is in scope.
func TestAttachIdentity_NoIdentityNoResolver(t *testing.T) {
	called := 0
	swapResolverFactory(t, func(_ authtypes.AuthManager, _ *schema.ConfigAndStacksInfo) store.AuthContextResolver {
		called++
		return stubResolver{}
	})

	info := &schema.ConfigAndStacksInfo{}
	artOpts := artifact.StoreOptions{}
	attachIdentity(&artOpts, info)

	assert.Empty(t, artOpts.Identity)
	assert.Nil(t, artOpts.Resolver)
	assert.Equal(t, 0, called)
}

// TestAttachIdentity_NilInfoIsSafe verifies attachIdentity does not panic
// when called without an info struct.
func TestAttachIdentity_NilInfoIsSafe(t *testing.T) {
	artOpts := artifact.StoreOptions{}
	require.NotPanics(t, func() {
		attachIdentity(&artOpts, nil)
	})
	assert.Empty(t, artOpts.Identity)
	assert.Nil(t, artOpts.Resolver)
}

// TestAttachIdentity_NoAuthManagerLeavesResolverNil verifies the identity is
// still recorded but no resolver is built when AuthManager is unavailable.
func TestAttachIdentity_NoAuthManagerLeavesResolverNil(t *testing.T) {
	swapResolverFactory(t, func(_ authtypes.AuthManager, _ *schema.ConfigAndStacksInfo) store.AuthContextResolver {
		t.Fatal("factory must not be called when AuthManager is nil")
		return nil
	})

	info := &schema.ConfigAndStacksInfo{Identity: "deploy"}
	artOpts := artifact.StoreOptions{}
	attachIdentity(&artOpts, info)

	assert.Equal(t, "deploy", artOpts.Identity)
	assert.Nil(t, artOpts.Resolver)
}

// TestAttachIdentity_AuthManagerWrongTypeFallsBack guards against panics if
// info.AuthManager holds an unexpected type.
func TestAttachIdentity_AuthManagerWrongTypeFallsBack(t *testing.T) {
	swapResolverFactory(t, func(_ authtypes.AuthManager, _ *schema.ConfigAndStacksInfo) store.AuthContextResolver {
		t.Fatal("factory must not be called when AuthManager has wrong type")
		return nil
	})

	info := &schema.ConfigAndStacksInfo{
		Identity:    "deploy",
		AuthManager: "not-an-auth-manager",
	}
	artOpts := artifact.StoreOptions{}
	require.NotPanics(t, func() {
		attachIdentity(&artOpts, info)
	})
	assert.Equal(t, "deploy", artOpts.Identity)
	assert.Nil(t, artOpts.Resolver)
}

// TestAttachStoreIdentity_StoreOverridesComponent is the issue #3400 regression
// guard: a store-level identity must take precedence over the component's
// active identity, AND the resolver must be attached so a non-ambient identity
// is actually resolved rather than silently falling back to the default chain.
func TestAttachStoreIdentity_StoreOverridesComponent(t *testing.T) {
	called := 0
	swapResolverFactory(t, func(_ authtypes.AuthManager, _ *schema.ConfigAndStacksInfo) store.AuthContextResolver {
		called++
		return stubResolver{}
	})

	info := &schema.ConfigAndStacksInfo{
		Identity:    "azure-plan", // the component's (non-AWS) identity
		AuthManager: &stubAuthManager{},
	}
	artOpts := artifact.StoreOptions{}
	attachStoreIdentity(&artOpts, info, "aws-backend")

	assert.Equal(t, "aws-backend", artOpts.Identity, "store-level identity must win over the component identity")
	assert.NotNil(t, artOpts.Resolver, "resolver must be attached so the store identity can be resolved")
	assert.Equal(t, 1, called)
}

// TestAttachStoreIdentity_FallsBackToComponentIdentity verifies the prior
// behavior is preserved when no store-level identity is configured.
func TestAttachStoreIdentity_FallsBackToComponentIdentity(t *testing.T) {
	swapResolverFactory(t, func(_ authtypes.AuthManager, _ *schema.ConfigAndStacksInfo) store.AuthContextResolver {
		return stubResolver{}
	})

	info := &schema.ConfigAndStacksInfo{
		Identity:    "ci-deployer",
		AuthManager: &stubAuthManager{},
	}
	artOpts := artifact.StoreOptions{}
	attachStoreIdentity(&artOpts, info, "")

	assert.Equal(t, "ci-deployer", artOpts.Identity)
	assert.NotNil(t, artOpts.Resolver)
}

// TestAttachStoreIdentity_StoreIdentityWithoutComponentIdentity covers the gap
// the naive fix would miss: when the component declares no identity but the
// store does, the store identity and its resolver must still be attached. The
// old early-return on an empty component identity would have dropped a
// store-only identity to the default credential chain.
func TestAttachStoreIdentity_StoreIdentityWithoutComponentIdentity(t *testing.T) {
	called := 0
	swapResolverFactory(t, func(_ authtypes.AuthManager, _ *schema.ConfigAndStacksInfo) store.AuthContextResolver {
		called++
		return stubResolver{}
	})

	info := &schema.ConfigAndStacksInfo{AuthManager: &stubAuthManager{}} // no component Identity
	artOpts := artifact.StoreOptions{}
	attachStoreIdentity(&artOpts, info, "aws-backend")

	assert.Equal(t, "aws-backend", artOpts.Identity)
	assert.NotNil(t, artOpts.Resolver)
	assert.Equal(t, 1, called)
}

// TestAttachStoreIdentity_NoIdentityAnywhere verifies nothing is attached when
// neither the store nor the component names an identity.
func TestAttachStoreIdentity_NoIdentityAnywhere(t *testing.T) {
	swapResolverFactory(t, func(_ authtypes.AuthManager, _ *schema.ConfigAndStacksInfo) store.AuthContextResolver {
		t.Fatal("factory must not be called when no identity is in scope")
		return nil
	})

	info := &schema.ConfigAndStacksInfo{}
	artOpts := artifact.StoreOptions{}
	attachStoreIdentity(&artOpts, info, "")

	assert.Empty(t, artOpts.Identity)
	assert.Nil(t, artOpts.Resolver)
}

// TestAttachStoreIdentity_NilInfoIsSafe verifies attachStoreIdentity does not
// panic when info is nil (no auth manager, so nothing to attach).
func TestAttachStoreIdentity_NilInfoIsSafe(t *testing.T) {
	artOpts := artifact.StoreOptions{}
	require.NotPanics(t, func() {
		attachStoreIdentity(&artOpts, nil, "aws-backend")
	})
	assert.Empty(t, artOpts.Identity)
	assert.Nil(t, artOpts.Resolver)
}
