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

func TestEmulatorNetworkAliasScopesByStack(t *testing.T) {
	assert.Equal(t, "dev-aws", emulatorNetworkAlias("dev", "aws"))
	assert.Equal(t, "deploy-prod-aws", emulatorNetworkAlias("deploy/prod", "aws"))
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

	// Force the shared-network path even when the test binary itself runs inside
	// a container (e.g. CI), where attachSharedNetwork would otherwise join the
	// runner's own network instead of ensuring the per-stack one.
	t.Setenv(envEmulatorUseCurrentContainerNetwork, "false")

	t.Run("attaches network with stack-scoped alias on success", func(t *testing.T) {
		rt := &fakeNetworkRuntime{}
		namedConfig := &container.NamedConfig{}
		m.attachSharedNetwork(context.Background(), rt, namedConfig, "dev", "gitserver")
		assert.Equal(t, []string{"atmos-emulator-dev"}, rt.ensured)
		assert.Equal(t, []container.NetworkAttachment{
			{Name: "atmos-emulator-dev", Aliases: []string{"dev-gitserver"}},
		}, namedConfig.Networks)
	})

	t.Run("network creation failure leaves networks unchanged", func(t *testing.T) {
		rt := &fakeNetworkRuntime{err: errors.New("network create failed")}
		namedConfig := &container.NamedConfig{}
		m.attachSharedNetwork(context.Background(), rt, namedConfig, "dev", "gitserver")
		assert.Empty(t, namedConfig.Networks)
	})

	t.Run("runtime without NetworkEnsurer is a no-op", func(t *testing.T) {
		namedConfig := &container.NamedConfig{}
		m.attachSharedNetwork(context.Background(), &plainRuntime{}, namedConfig, "dev", "gitserver")
		assert.Empty(t, namedConfig.Networks)
	})
}
