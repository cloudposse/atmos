package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// fakeEmulatorResolver is a tiny test double implementing types.EmulatorResolver.
type fakeEmulatorResolver struct {
	env        map[string]string
	kubeconfig []byte
}

func (f *fakeEmulatorResolver) ResolveEmulator(_ context.Context, _ string) (map[string]string, []byte, error) {
	return f.env, f.kubeconfig, nil
}

// saveAndRestoreResolver snapshots the process-wide resolver and restores it on
// cleanup so these tests do not leak global state into the rest of the package.
func saveAndRestoreResolver(t *testing.T) {
	t.Helper()
	prev := defaultEmulatorResolver
	t.Cleanup(func() { defaultEmulatorResolver = prev })
}

// TestSetEmulatorResolver_RoundTrip verifies that setting a resolver makes it the
// process-wide resolver, returning the exact instance that was registered.
func TestSetEmulatorResolver_RoundTrip(t *testing.T) {
	saveAndRestoreResolver(t)

	want := &fakeEmulatorResolver{env: map[string]string{"AWS_ENDPOINT_URL": "http://localhost:1"}}
	SetEmulatorResolver(want)

	require.NotNil(t, defaultEmulatorResolver)
	assert.Same(t, want, defaultEmulatorResolver, "the exact registered resolver instance is stored")

	// The stored resolver behaves like the registered one.
	env, kubeconfig, err := defaultEmulatorResolver.ResolveEmulator(context.Background(), "local/aws")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:1", env["AWS_ENDPOINT_URL"], "registered resolver's env round-trips")
	assert.Nil(t, kubeconfig)
}

// TestSetEmulatorResolver_OverwritesPrevious verifies a later registration
// replaces an earlier one (last writer wins).
func TestSetEmulatorResolver_OverwritesPrevious(t *testing.T) {
	saveAndRestoreResolver(t)

	first := &fakeEmulatorResolver{env: map[string]string{"K": "first"}}
	second := &fakeEmulatorResolver{env: map[string]string{"K": "second"}}

	SetEmulatorResolver(first)
	SetEmulatorResolver(second)

	assert.Same(t, second, defaultEmulatorResolver, "last registered resolver wins")
	env, _, err := defaultEmulatorResolver.ResolveEmulator(context.Background(), "local/aws")
	require.NoError(t, err)
	assert.Equal(t, "second", env["K"])
}

// TestSetEmulatorResolver_NilClearsResolver verifies the unset/default behavior:
// registering nil leaves no resolver (the state contexts that never import the
// emulator component start in).
func TestSetEmulatorResolver_NilClearsResolver(t *testing.T) {
	saveAndRestoreResolver(t)

	SetEmulatorResolver(&fakeEmulatorResolver{})
	require.NotNil(t, defaultEmulatorResolver)

	SetEmulatorResolver(nil)
	assert.Nil(t, defaultEmulatorResolver, "registering nil clears the process-wide resolver")
}

// stubResolverReceiver implements the emulatorResolverReceiver interface so the
// interface contract (the manager injects the resolver) is exercised.
type stubResolverReceiver struct {
	gotResolver types.EmulatorResolver
}

func (s *stubResolverReceiver) SetEmulatorResolver(r types.EmulatorResolver) { s.gotResolver = r }

// TestEmulatorResolverReceiver_Contract verifies an identity implementing the
// emulatorResolverReceiver seam receives the injected resolver.
func TestEmulatorResolverReceiver_Contract(t *testing.T) {
	resolver := &fakeEmulatorResolver{}
	var receiver emulatorResolverReceiver = &stubResolverReceiver{}

	receiver.SetEmulatorResolver(resolver)

	stub, ok := receiver.(*stubResolverReceiver)
	require.True(t, ok)
	assert.Same(t, resolver, stub.gotResolver, "injected resolver reaches the receiver")
}
