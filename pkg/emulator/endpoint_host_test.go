package emulator

import (
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

func TestParseLinuxDefaultGateway(t *testing.T) {
	routeTable := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0102A8C0\t0003\t0\t0\t0\t00000000\t0\t0\t0\n"

	assert.Equal(t, "192.168.2.1", parseLinuxDefaultGateway(routeTable))
	assert.Empty(t, parseLinuxDefaultGateway("Iface Destination Gateway\neth0 00000001 0102A8C0\n"))
}

// stubEndpointHostDetection fakes containerization detection (now delegated to
// container.ProcessRunsInContainer, an exported var directly reassignable by
// any importing package's tests) and the /proc/net/route reader this package
// still owns for gateway-guessing.
func stubEndpointHostDetection(t *testing.T, inContainer bool, routeTable string) func() {
	t.Helper()

	origProcessRunsInContainer := container.ProcessRunsInContainer
	origReadProcFile := readProcFile

	container.ProcessRunsInContainer = func() bool { return inContainer }
	readProcFile = func(name string) ([]byte, error) {
		if name == "/proc/net/route" {
			return []byte(routeTable), nil
		}
		return nil, os.ErrNotExist
	}

	return func() {
		container.ProcessRunsInContainer = origProcessRunsInContainer
		readProcFile = origReadProcFile
	}
}
