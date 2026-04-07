package client

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The ScopedAuthProvider is a thin MCP adapter over
// auth.CreateAndAuthenticateManagerWithEnvOverrides. Its *only* job is:
//
//   1. Plumb ParsedConfig.Env into the pkg/auth primitive.
//   2. Convert (nil, nil) into errUtils.ErrMCPServerAuthUnavailable carrying
//      server+identity context (errors.Is-matchable).
//   3. Pass through underlying errors from the builder unchanged. Higher
//      layers (Session.Start) wrap with errUtils.ErrMCPServerStartFailed and
//      the server name; the adapter must not duplicate that context.
//
// These tests therefore stub buildManagerFn directly — the env-override,
// config-reload, and auth-manager construction semantics are covered by the
// pkg/auth tests, not here. Duplicating them would only create fragile
// cross-package coupling.

// Static sentinel errors for test injection. These are package-level vars,
// not inline errors.New calls, so they comply with CLAUDE.md's "no dynamic
// errors" rule and are matchable via errors.Is in assertions.
var (
	errTestScopedBuilderInit = errors.New("test: simulated builder init failure")
)

// fakeMgrImpl is a minimal auth.AuthManager stand-in.
type fakeMgrImpl struct {
	auth.AuthManager // embed nil — only PrepareShellEnvironment is exercised.
}

func (f *fakeMgrImpl) PrepareShellEnvironment(_ context.Context, identityName string, currentEnv []string) ([]string, error) {
	return append(currentEnv, "FAKE="+identityName), nil
}

// withStubbedBuilder overrides ScopedAuthProvider.buildManagerFn with a fake
// and records the env map it was called with.
func withStubbedBuilder(
	p *ScopedAuthProvider,
	capturedEnv *map[string]string,
	mgr auth.AuthManager,
	err error,
) {
	p.buildManagerFn = func(envOverrides map[string]string) (auth.AuthManager, error) {
		if capturedEnv != nil {
			*capturedEnv = envOverrides
		}
		return mgr, err
	}
}

func TestScopedAuthProvider_ForServer_PlumbsServerEnvToBuilder(t *testing.T) {
	p := NewScopedAuthProvider(&schema.AtmosConfiguration{})
	var captured map[string]string
	withStubbedBuilder(p, &captured, &fakeMgrImpl{}, nil)

	cfg := &ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
		Env: map[string]string{
			"ATMOS_PROFILE": "managers",
		},
	}

	mgr, err := p.ForServer(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, mgr)

	// The adapter's sole contract: it passes ParsedConfig.Env to the builder.
	assert.Equal(t, cfg.Env, captured)
}

func TestScopedAuthProvider_ForServer_NilManager_ReturnsSentinel(t *testing.T) {
	p := NewScopedAuthProvider(&schema.AtmosConfiguration{})
	// (nil, nil) simulates auth disabled or no identity resolved.
	withStubbedBuilder(p, nil, nil, nil)

	cfg := &ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
	}

	mgr, err := p.ForServer(context.Background(), cfg)
	require.Error(t, err)
	assert.Nil(t, mgr)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerAuthUnavailable)
	assert.Contains(t, err.Error(), "atmos", "error must mention the server name")
	assert.Contains(t, err.Error(), "core-root/terraform", "error must mention the identity")
}

func TestScopedAuthProvider_ForServer_BuilderError_PassesThrough(t *testing.T) {
	// Contract: the adapter passes underlying builder errors through unchanged.
	// Higher layers (Session.Start) add ErrMCPServerStartFailed and the server
	// name, so the adapter must not duplicate that context.
	p := NewScopedAuthProvider(&schema.AtmosConfiguration{})
	withStubbedBuilder(p, nil, nil, errTestScopedBuilderInit)

	cfg := &ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
	}

	mgr, err := p.ForServer(context.Background(), cfg)
	require.Error(t, err)
	assert.Nil(t, mgr)
	// The static sentinel must still match via errors.Is.
	assert.ErrorIs(t, err, errTestScopedBuilderInit)
	// And it must be the *exact* error returned from the builder — no
	// adapter-side wrapping.
	assert.Equal(t, errTestScopedBuilderInit, err, "adapter must pass builder errors through unchanged")
}

func TestScopedAuthProvider_ForServer_NilConfig_ReturnsSentinel(t *testing.T) {
	// PerServerAuthProvider is a public interface; passing nil from an
	// external caller must surface a typed error rather than panic.
	p := NewScopedAuthProvider(&schema.AtmosConfiguration{})
	// Spy on the builder so we can prove it was never called when config is nil.
	builderInvoked := false
	withStubbedBuilder(p, nil, &fakeMgrImpl{}, nil)
	origBuilder := p.buildManagerFn
	p.buildManagerFn = func(envOverrides map[string]string) (auth.AuthManager, error) {
		builderInvoked = true
		return origBuilder(envOverrides)
	}

	mgr, err := p.ForServer(context.Background(), nil)
	require.Error(t, err)
	assert.Nil(t, mgr)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerAuthUnavailable)
	assert.Contains(t, err.Error(), "nil server config")
	assert.False(t, builderInvoked, "builder must not be invoked when config is nil — guard must short-circuit")
}

func TestScopedAuthProvider_PrepareShellEnvironment_FallbackPath(t *testing.T) {
	p := NewScopedAuthProvider(&schema.AtmosConfiguration{})
	var captured map[string]string
	withStubbedBuilder(p, &captured, &fakeMgrImpl{}, nil)

	out, err := p.PrepareShellEnvironment(
		context.Background(),
		"core-root/terraform",
		[]string{"PATH=/usr/bin"},
	)
	require.NoError(t, err)
	assert.Nil(t, captured, "fallback path must call builder with nil overrides")
	assert.Contains(t, out, "FAKE=core-root/terraform")
	assert.Contains(t, out, "PATH=/usr/bin")
}

func TestScopedAuthProvider_PrepareShellEnvironment_NilManager_ReturnsSentinel(t *testing.T) {
	p := NewScopedAuthProvider(&schema.AtmosConfiguration{})
	withStubbedBuilder(p, nil, nil, nil)

	out, err := p.PrepareShellEnvironment(
		context.Background(),
		"core-root/terraform",
		[]string{"PATH=/usr/bin"},
	)
	require.Error(t, err)
	assert.Nil(t, out)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerAuthUnavailable)
	assert.Contains(t, err.Error(), "core-root/terraform")
}

func TestScopedAuthProvider_PrepareShellEnvironment_BuilderError(t *testing.T) {
	p := NewScopedAuthProvider(&schema.AtmosConfiguration{})
	withStubbedBuilder(p, nil, nil, errTestScopedBuilderInit)

	_, err := p.PrepareShellEnvironment(
		context.Background(),
		"core-root/terraform",
		[]string{"PATH=/usr/bin"},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTestScopedBuilderInit)
}

func TestScopedAuthProvider_ImplementsBothInterfaces(t *testing.T) {
	var p AuthEnvProvider = NewScopedAuthProvider(&schema.AtmosConfiguration{})
	_, ok := p.(PerServerAuthProvider)
	assert.True(t, ok, "ScopedAuthProvider must implement PerServerAuthProvider")
}

func TestNewScopedAuthProvider_DefaultBuilderWiredToAuthPackage(t *testing.T) {
	// Smoke test that the default constructor wires buildManagerFn to the
	// pkg/auth primitive. We don't invoke it (that would need real atmos
	// config) — just verify it's not nil and is the expected function.
	p := NewScopedAuthProvider(&schema.AtmosConfiguration{})
	require.NotNil(t, p.buildManagerFn)

	// The zero value behavior check: after construction the builder exists,
	// meaning a caller that bypasses the test hooks would get real behavior.
	// (We cannot compare function values for equality in Go.) This assertion
	// is therefore just a presence check.
	_ = auth.CreateAndAuthenticateManagerWithEnvOverrides
}
