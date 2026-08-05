package emulator

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/container"
)

func TestReachableHostForPublishedPorts_HostNativeUsesLocalhost(t *testing.T) {
	t.Setenv(envEmulatorEndpointHost, "")
	restore := stubEndpointHostDetection(t, false, "")

	got := reachableHostForPublishedPorts()

	assert.Equal(t, "localhost", got)
	restore()
}

func TestReachableHostForPublishedPorts_ContainerUsesDefaultGateway(t *testing.T) {
	t.Setenv(envEmulatorEndpointHost, "")
	restore := stubEndpointHostDetection(t, true, "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\neth0\t00000000\t010011AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n")

	got := reachableHostForPublishedPorts()

	assert.Equal(t, "172.17.0.1", got)
	restore()
}

func TestReachableHostForPublishedPorts_OverrideWins(t *testing.T) {
	t.Setenv(envEmulatorEndpointHost, "host.docker.internal")
	restore := stubEndpointHostDetection(t, false, "")

	got := reachableHostForPublishedPorts()

	assert.Equal(t, "host.docker.internal", got)
	restore()
}

func TestFirstReachableNetwork(t *testing.T) {
	assert.Equal(t, "github_network_123", firstReachableNetwork([]string{"", "none", "host", "github_network_123"}))
	assert.Empty(t, firstReachableNetwork([]string{"", "none", "host"}))
}

func TestUseCurrentContainerNetworkRequiresActionsOrOverride(t *testing.T) {
	restore := stubEndpointHostDetection(t, true, "")
	defer restore()

	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv(envEmulatorUseCurrentContainerNetwork, "")
	assert.False(t, useCurrentContainerNetwork())

	t.Setenv("GITHUB_ACTIONS", "true")
	assert.True(t, useCurrentContainerNetwork())

	t.Setenv(envEmulatorUseCurrentContainerNetwork, "false")
	assert.False(t, useCurrentContainerNetwork())

	t.Setenv(envEmulatorUseCurrentContainerNetwork, "true")
	assert.True(t, useCurrentContainerNetwork())
}

func TestParseLinuxDefaultGateway(t *testing.T) {
	routeTable := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0102A8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n"

	assert.Equal(t, "192.168.2.1", parseLinuxDefaultGateway(routeTable))
	assert.Empty(t, parseLinuxDefaultGateway("Iface Destination Gateway\neth0 00000001 0102A8C0\n"))
}

type staticInspectRuntime struct {
	container.Runtime
	info *container.Info
}

func (r staticInspectRuntime) Inspect(context.Context, string) (*container.Info, error) {
	return r.info, nil
}

func TestCurrentContainerNetwork(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv(envEmulatorUseCurrentContainerNetwork, "")
	restore := stubEndpointHostDetection(t, true, "")
	defer restore()

	got := currentContainerNetwork(context.Background(), staticInspectRuntime{
		info: &container.Info{Networks: []string{"none", "github_network_123"}},
	})

	assert.Equal(t, "github_network_123", got)
}

func stubEndpointHostDetection(t *testing.T, inContainer bool, routeTable string) func() {
	t.Helper()

	origProcessRunsInContainer := processRunsInContainer
	origReadProcFile := readProcFile

	processRunsInContainer = func() bool { return inContainer }
	readProcFile = func(name string) ([]byte, error) {
		if name == "/proc/net/route" {
			return []byte(routeTable), nil
		}
		return nil, os.ErrNotExist
	}

	return func() {
		processRunsInContainer = origProcessRunsInContainer
		readProcFile = origReadProcFile
	}
}
