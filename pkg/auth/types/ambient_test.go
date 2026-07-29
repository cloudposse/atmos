package types

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubProvider implements Provider without implementing AmbientProvider. It stands in for
// every provider that predates the ambient contract and must keep caching normally.
type stubProvider struct{}

// The methods below exist only to satisfy Provider. None of them are exercised: the
// tests in this file call ProviderIsAmbient, which inspects the type rather than
// invoking it. They return zero values deliberately, so any accidental reliance on
// this double doing real work fails loudly rather than passing on fake data.

// Kind identifies the double.
func (stubProvider) Kind() string { return "test" }

// Name identifies the double.
func (stubProvider) Name() string { return "test" }

// PreAuthenticate is a no-op.
func (stubProvider) PreAuthenticate(_ AuthManager) error { return nil }

// Authenticate mints nothing.
func (stubProvider) Authenticate(_ context.Context) (ICredentials, error) {
	return nil, nil
}

// Validate accepts the configuration unconditionally.
func (stubProvider) Validate() error { return nil }

// Environment contributes no variables.
func (stubProvider) Environment() (map[string]string, error) { return nil, nil }

// Paths reports no credential files.
func (stubProvider) Paths() ([]Path, error) { return nil, nil }

// PrepareEnvironment returns the environment unchanged.
func (stubProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}

// Logout has nothing to clean up.
func (stubProvider) Logout(_ context.Context) error { return nil }

// GetFilesDisplayPath reports no managed files.
func (stubProvider) GetFilesDisplayPath() string { return "" }

// SetRealm is a no-op; this double is realm-independent.
func (stubProvider) SetRealm(_ string) {}

// ambientStubProvider additionally implements AmbientProvider, with the answer under the
// test's control so both directions of the opt-in are exercised.
type ambientStubProvider struct {
	stubProvider
	ambient bool
}

// IsAmbient returns the test-controlled answer, so both directions of the opt-in are
// exercised through the same double.
func (p ambientStubProvider) IsAmbient() bool { return p.ambient }

var (
	_ Provider        = stubProvider{}
	_ AmbientProvider = ambientStubProvider{}
)

// TestProviderIsAmbient covers the helper's four cases: a nil provider, one that does
// not implement the optional interface, and both answers from one that does.
func TestProviderIsAmbient(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		want     bool
	}{
		{
			name:     "nil provider",
			provider: nil,
			want:     false,
		},
		{
			name:     "provider that does not implement AmbientProvider",
			provider: stubProvider{},
			want:     false,
		},
		{
			name:     "provider opting in",
			provider: ambientStubProvider{ambient: true},
			want:     true,
		},
		{
			// Implementing the interface is not enough — a provider that answers false
			// must be cached exactly like one that never implemented it.
			name:     "provider implementing the interface but opting out",
			provider: ambientStubProvider{ambient: false},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ProviderIsAmbient(tt.provider))
		})
	}
}
