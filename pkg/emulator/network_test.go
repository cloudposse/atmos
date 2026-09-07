package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/container"
)

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

// TestManagerAttachSharedNetwork_DelegatesToContainerPackage proves the
// Manager's attachSharedNetwork is a thin wrapper over the shared
// container.AttachSharedNetwork -- the naming/attach behavior itself is
// exhaustively tested in pkg/container/stack_network_test.go, including the
// cross-kind unification with components.container.
func TestManagerAttachSharedNetwork_DelegatesToContainerPackage(t *testing.T) {
	// Force the shared-network path even when the test binary itself runs inside
	// a container (e.g. CI).
	t.Setenv("ATMOS_EMULATOR_USE_CURRENT_CONTAINER_NETWORK", "false")

	m := &Manager{}
	rt := &fakeNetworkRuntime{}
	namedConfig := &container.NamedConfig{}

	m.attachSharedNetwork(context.Background(), rt, namedConfig, "dev", "gitserver")

	assert.Equal(t, []string{"atmos-dev"}, rt.ensured)
	assert.Equal(t, []container.NetworkAttachment{
		{Name: "atmos-dev", Aliases: []string{"dev-gitserver"}},
	}, namedConfig.Networks)
}
