package tests

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

// registryHostPort is the fixed host port the emulator-registry fixture publishes
// the registry on (see the component's container.ports). It is deliberately non-standard:
// the conventional 5000 is reserved by macOS (AirPlay Receiver). The container serves on
// 5000; only the host mapping differs.
const registryHostPort = "15500"

// TestEmulatorRegistryLifecycle proves the OCI / Terraform registry emulator comes
// up and serves the registry API via the emulator-component path: `atmos emulator up
// registry` starts the standard registry:2 container (atmos manages it, not a pre-started
// service), and the OCI distribution API answers on the published port.
//
// Opt-in and runtime-gated like the other emulator E2Es: it runs only when
// ATMOS_TEST_FLOCI=true and a container runtime (Docker or Podman) is available.
func TestEmulatorRegistryLifecycle(t *testing.T) {
	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true to run the emulator E2E")
	}

	ctx := context.Background()
	if _, err := container.DetectRuntime(ctx); err != nil {
		t.Skipf("no container runtime available for the emulator E2E: %v", err)
	}

	ensureAtmosRunner(t)

	workdir := copyFlociScenarioFixture(t, filepath.Join("fixtures", "scenarios", "emulator-registry"))

	env := map[string]string{
		"ATMOS_CLI_CONFIG_PATH": workdir,
		"TF_IN_AUTOMATION":      "1",
		// Persistence is enabled by default; keep this test's state in a temp dir so
		// it never writes to the developer's real XDG cache.
		"ATMOS_XDG_CACHE_HOME": t.TempDir(),
	}
	const stack = "local"

	run := func(timeout time.Duration, args ...string) (string, string, error) {
		return runFlociAtmosInDir(t, workdir, env, timeout, args...)
	}

	// Start the registry; always tear the container down afterward.
	_, stderr, err := run(3*time.Minute, "emulator", "up", "registry", "-s", stack)
	require.NoErrorf(t, err, "emulator up failed: %s", stderr)
	t.Cleanup(func() {
		_, _, _ = run(2*time.Minute, "emulator", "down", "registry", "-s", stack)
	})

	// The OCI distribution API answers 200 at /v2/ once the registry is serving.
	v2URL := "http://localhost:" + registryHostPort + "/v2/"
	require.Eventually(t, func() bool {
		probeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, reqErr := http.NewRequestWithContext(probeCtx, http.MethodGet, v2URL, nil)
		if reqErr != nil {
			return false
		}
		resp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 90*time.Second, 2*time.Second, "registry OCI API did not become ready at /v2/")
}
