package tests

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

// runEmulatorTCPLifecycle brings an emulator up via the emulator-component path, waits until it
// accepts TCP connections on the pinned host port, and tears it down. It is the shared
// body of the cloud-API emulator lifecycle E2Es (GCP, Azure), whose readiness is best
// probed at the TCP layer rather than a product-specific HTTP route.
//
// Opt-in and runtime-gated like the other emulator E2Es: it runs only when
// ATMOS_TEST_FLOCI=true and a container runtime (Docker or Podman) is available.
func runEmulatorTCPLifecycle(t *testing.T, fixture, emulatorName, hostPort string, readiness time.Duration) {
	t.Helper()

	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true to run the emulator E2E")
	}
	if _, err := container.DetectRuntime(context.Background()); err != nil {
		t.Skipf("no container runtime available for the emulator E2E: %v", err)
	}

	ensureAtmosRunner(t)

	workdir := copyFlociScenarioFixture(t, filepath.Join("fixtures", "scenarios", fixture))
	env := map[string]string{
		"ATMOS_CLI_CONFIG_PATH": workdir,
		"TF_IN_AUTOMATION":      "1",
	}
	const stack = "local"

	run := func(timeout time.Duration, args ...string) (string, string, error) {
		return runFlociAtmosInDir(t, workdir, env, timeout, args...)
	}

	// Start the emulator; always tear the container down afterward.
	_, stderr, err := run(3*time.Minute, "emulator", "up", emulatorName, "-s", stack)
	require.NoErrorf(t, err, "emulator up failed: %s", stderr)
	t.Cleanup(func() {
		_, _, _ = run(2*time.Minute, "emulator", "down", emulatorName, "-s", stack)
	})

	// The emulator is ready once it accepts connections on the published host port.
	addr := net.JoinHostPort("localhost", hostPort)
	require.Eventually(t, func() bool {
		conn, dialErr := net.DialTimeout("tcp", addr, 2*time.Second)
		if dialErr != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, readiness, 2*time.Second, "emulator %q did not start listening on %s", emulatorName, addr)
}

// TestEmulatorGCPLifecycle exercises `atmos emulator up gcp` (floci/gcp) end to end:
// atmos manages the container and the GCP emulator endpoint accepts connections. It
// complements TestGCPSecretsFlociE2E (which uses a pre-started Floci service container) by
// covering the emulator-component lifecycle.
func TestEmulatorGCPLifecycle(t *testing.T) {
	runEmulatorTCPLifecycle(t, "emulator-gcp", "gcp", "14588", 90*time.Second)
}

// TestEmulatorAzureLifecycle exercises `atmos emulator up azure` (floci/az) end to
// end, complementing TestAzureSecretsFlociE2E.
func TestEmulatorAzureLifecycle(t *testing.T) {
	runEmulatorTCPLifecycle(t, "emulator-azure", "azure", "14577", 90*time.Second)
}
