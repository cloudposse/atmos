package emulator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/container"
)

func TestEmulatorNetworkName(t *testing.T) {
	cases := map[string]string{
		"local":       "atmos-emulator-local",
		"deploy/prod": "atmos-emulator-deploy-prod",
		"a b":         "atmos-emulator-a-b",
		"":            "atmos-emulator-default",
	}
	for in, want := range cases {
		assert.Equal(t, want, emulatorNetworkName(in), "stack %q", in)
	}
}

func TestSanitizeNetworkToken(t *testing.T) {
	// Allowed characters pass through; everything else collapses to '-'.
	assert.Equal(t, "ue2-prod_1.2", sanitizeNetworkToken("ue2-prod_1.2"))
	assert.Equal(t, "x--y", sanitizeNetworkToken("x/?y"))
	assert.Equal(t, "default", sanitizeNetworkToken(""))
}

// fakeNetworkRuntime is a container.Runtime that also implements
// container.NetworkEnsurer, recording EnsureNetwork calls.
type fakeNetworkRuntime struct {
	container.Runtime
	ensured []string
	err     error
}

func (f *fakeNetworkRuntime) EnsureNetwork(_ context.Context, name string) error {
	f.ensured = append(f.ensured, name)
	return f.err
}

// plainRuntime is a container.Runtime without the NetworkEnsurer capability.
type plainRuntime struct {
	container.Runtime
}

func TestAttachSharedNetwork(t *testing.T) {
	m := &Manager{}

	t.Run("attaches network and alias on success", func(t *testing.T) {
		rt := &fakeNetworkRuntime{}
		namedConfig := &container.NamedConfig{}
		m.attachSharedNetwork(context.Background(), rt, namedConfig, "dev", "gitserver")
		assert.Equal(t, []string{"atmos-emulator-dev"}, rt.ensured)
		assert.Equal(t, []string{"--network", "atmos-emulator-dev", "--network-alias", "gitserver"}, namedConfig.RunArgs)
	})

	t.Run("network creation failure leaves run args unchanged", func(t *testing.T) {
		rt := &fakeNetworkRuntime{err: errors.New("network create failed")}
		namedConfig := &container.NamedConfig{}
		m.attachSharedNetwork(context.Background(), rt, namedConfig, "dev", "gitserver")
		assert.Empty(t, namedConfig.RunArgs)
	})

	t.Run("runtime without NetworkEnsurer is a no-op", func(t *testing.T) {
		namedConfig := &container.NamedConfig{}
		m.attachSharedNetwork(context.Background(), &plainRuntime{}, namedConfig, "dev", "gitserver")
		assert.Empty(t, namedConfig.RunArgs)
	})
}
