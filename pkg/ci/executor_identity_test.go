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
