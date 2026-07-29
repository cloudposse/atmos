package types

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubProvider implements Provider without implementing AmbientProvider. It stands in for
// every provider that predates the ambient contract and must keep caching normally.
type stubProvider struct{}

func (stubProvider) Kind() string                        { return "test" }
func (stubProvider) Name() string                        { return "test" }
func (stubProvider) PreAuthenticate(_ AuthManager) error { return nil }
func (stubProvider) Authenticate(_ context.Context) (ICredentials, error) {
	return nil, nil
}
func (stubProvider) Validate() error                         { return nil }
func (stubProvider) Environment() (map[string]string, error) { return nil, nil }
func (stubProvider) Paths() ([]Path, error)                  { return nil, nil }
func (stubProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}
func (stubProvider) Logout(_ context.Context) error { return nil }
func (stubProvider) GetFilesDisplayPath() string    { return "" }
func (stubProvider) SetRealm(_ string)              {}

// ambientStubProvider additionally implements AmbientProvider, with the answer under the
// test's control so both directions of the opt-in are exercised.
type ambientStubProvider struct {
	stubProvider
	ambient bool
}

func (p ambientStubProvider) IsAmbient() bool { return p.ambient }

var (
	_ Provider        = stubProvider{}
	_ AmbientProvider = ambientStubProvider{}
)

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
