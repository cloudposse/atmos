package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRegistry_BuiltinTracksRegistered verifies the store and sops providers
// self-register via their init() blocks when the package loads.
func TestRegistry_BuiltinTracksRegistered(t *testing.T) {
	assert.Equal(t, []string{TrackSops, TrackStore}, Registered())
}

// TestRegistry_NewDispatchesToStore confirms New routes the store track to the
// store-backed constructor (which fails because the store is unconfigured).
func TestRegistry_NewDispatchesToStore(t *testing.T) {
	_, err := New(&schema.AtmosConfiguration{}, TrackStore, "missing", nil)
	require.ErrorIs(t, err, ErrStoreNotFound)
}

// TestRegistry_NewDispatchesToSops confirms New routes the sops track to the
// SOPS constructor (which fails because the provider is unconfigured).
func TestRegistry_NewDispatchesToSops(t *testing.T) {
	_, err := New(&schema.AtmosConfiguration{}, TrackSops, "missing", nil)
	require.ErrorIs(t, err, ErrProviderNotFound)
}

// TestRegistry_NewUnknownTrack confirms an unregistered track is reported.
func TestRegistry_NewUnknownTrack(t *testing.T) {
	_, err := New(&schema.AtmosConfiguration{}, "nope", "x", nil)
	require.ErrorIs(t, err, ErrTrackNotRegistered)
}

// TestRegistry_DuplicateRegisterPanics confirms re-registering a track is a
// loud programming error.
func TestRegistry_DuplicateRegisterPanics(t *testing.T) {
	assert.PanicsWithError(t, ErrProviderAlreadyRegistered.Error()+": \""+TrackStore+"\"", func() {
		Register(TrackStore, func(*schema.AtmosConfiguration, string, map[string]any) (Provider, error) {
			return nil, nil
		})
	})
}
