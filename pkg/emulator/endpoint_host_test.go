package emulator

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

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
	defer restore()
	// Deterministic: this test asserts the gateway-guess tier specifically, so
	// host.docker.internal must not resolve, regardless of the real test host's
	// own DNS/hosts-file setup.
	restoreLookup := stubLookupHost(t, nil, errors.New("no such host"))
	defer restoreLookup()

	got := reachableHostForPublishedPorts()

	assert.Equal(t, "172.17.0.1", got)
}

func TestReachableHostForPublishedPorts_OverrideWins(t *testing.T) {
	t.Setenv(envEmulatorEndpointHost, "host.docker.internal")
	restore := stubEndpointHostDetection(t, false, "")

	got := reachableHostForPublishedPorts()

	assert.Equal(t, "host.docker.internal", got)
	restore()
}

func TestReachableHostForPublishedPorts_ContainerPrefersResolvableHostDockerInternal(t *testing.T) {
	t.Setenv(envEmulatorEndpointHost, "")
	restore := stubEndpointHostDetection(t, true, "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\neth0\t00000000\t010011AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n")
	defer restore()
	restoreLookup := stubLookupHost(t, []string{"192.168.65.2"}, nil)
	defer restoreLookup()

	got := reachableHostForPublishedPorts()

	assert.Equal(t, hostDockerInternal, got)
}

func TestReachableHostForPublishedPorts_ContainerFallsBackToGatewayWhenHostDockerInternalUnresolvable(t *testing.T) {
	t.Setenv(envEmulatorEndpointHost, "")
	restore := stubEndpointHostDetection(t, true, "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\neth0\t00000000\t010011AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n")
	defer restore()
	restoreLookup := stubLookupHost(t, nil, errors.New("no such host"))
	defer restoreLookup()

	got := reachableHostForPublishedPorts()

	assert.Equal(t, "172.17.0.1", got, "must fall back to the gateway guess -- unchanged behavior on native Linux Docker hosts -- when host.docker.internal doesn't resolve")
}

func TestHostDockerInternalResolves(t *testing.T) {
	t.Run("resolves", func(t *testing.T) {
		restore := stubLookupHost(t, []string{"192.168.65.2"}, nil)
		defer restore()
		assert.True(t, hostDockerInternalResolves())
	})

	t.Run("lookup error", func(t *testing.T) {
		restore := stubLookupHost(t, nil, errors.New("no such host"))
		defer restore()
		assert.False(t, hostDockerInternalResolves())
	})

	t.Run("no addresses", func(t *testing.T) {
		restore := stubLookupHost(t, nil, nil)
		defer restore()
		assert.False(t, hostDockerInternalResolves())
	})
}

// TestHostDockerInternalResolves_BoundedByTimeout proves a slow or
// unresponsive resolver can't delay endpoint construction indefinitely: the
// lookup's context must actually be cancelled once hostDockerInternalLookupTimeout
// elapses, not merely passed through unused.
func TestHostDockerInternalResolves_BoundedByTimeout(t *testing.T) {
	orig := lookupHost
	defer func() { lookupHost = orig }()

	// stubSafetyNet is a finite failure path independent of ctx cancellation --
	// well above hostDockerInternalLookupTimeout, but still bounded -- so a
	// regression that stops the production code from ever cancelling the
	// context fails this test with a clear timing mismatch instead of hanging
	// the suite indefinitely.
	const stubSafetyNet = hostDockerInternalLookupTimeout * 10
	lookupHost = func(ctx context.Context, _ string) ([]string, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(stubSafetyNet):
			return nil, context.DeadlineExceeded
		}
	}

	start := time.Now()
	got := hostDockerInternalResolves()
	elapsed := time.Since(start)

	assert.False(t, got)
	// Tight bound against the actual configured constant (not the safety net
	// above) -- this fails if the timeout stops being applied at all, since
	// elapsed would then jump to stubSafetyNet instead.
	assert.Less(t, elapsed, hostDockerInternalLookupTimeout+time.Second,
		"resolver context must actually be bounded by hostDockerInternalLookupTimeout")
}

// stubLookupHost overrides lookupHost for the duration of a test.
func stubLookupHost(t *testing.T, addrs []string, err error) func() {
	t.Helper()

	orig := lookupHost
	lookupHost = func(context.Context, string) ([]string, error) { return addrs, err }
	return func() { lookupHost = orig }
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
