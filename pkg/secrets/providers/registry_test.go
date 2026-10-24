package providers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	// Blank import registers the SOPS track so Registered()/New see it (the store track
	// self-registers from within the providers package).
	_ "github.com/cloudposse/atmos/pkg/secrets/providers/sops"
)

// TestRegistry_BuiltinTracksRegistered verifies the store and sops providers
// self-register via their init() blocks when the packages load.
func TestRegistry_BuiltinTracksRegistered(t *testing.T) {
	assert.Equal(t, []string{providers.TrackSops, providers.TrackStore}, providers.Registered())
}

// TestRegistry_NewDispatchesToStore confirms New routes the store track to the
// store-backed constructor (which fails because the store is unconfigured).
func TestRegistry_NewDispatchesToStore(t *testing.T) {
	_, err := providers.New(&schema.AtmosConfiguration{}, providers.TrackStore, "missing", nil)
	require.ErrorIs(t, err, providers.ErrStoreNotFound)
}

// TestRegistry_NewDispatchesToSops confirms New routes the sops track to the
// SOPS constructor (which fails because the provider is unconfigured).
func TestRegistry_NewDispatchesToSops(t *testing.T) {
	_, err := providers.New(&schema.AtmosConfiguration{}, providers.TrackSops, "missing", nil)
	require.ErrorIs(t, err, providers.ErrProviderNotFound)
}

// TestRegistry_NewUnknownTrack confirms an unregistered track is reported.
func TestRegistry_NewUnknownTrack(t *testing.T) {
	_, err := providers.New(&schema.AtmosConfiguration{}, "nope", "x", nil)
	require.ErrorIs(t, err, providers.ErrTrackNotRegistered)
}

// TestRegistry_DuplicateRegisterPanics confirms re-registering a track is a
// loud programming error.
func TestRegistry_DuplicateRegisterPanics(t *testing.T) {
	assert.PanicsWithError(t, providers.ErrProviderAlreadyRegistered.Error()+": \""+providers.TrackStore+"\"", func() {
		providers.Register(providers.TrackStore, func(*schema.AtmosConfiguration, string, map[string]any) (providers.Provider, error) {
			return nil, nil
		})
	})
}
