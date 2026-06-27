package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

// openBaoHostPort is the fixed host port the emulator-openbao fixture publishes
// the server on (see the component's container.ports). The atmos.yaml store points
// at this same address.
const openBaoHostPort = "8200"

// TestEmulatorOpenBaoLifecycle proves Atmos Secrets backed by Vault/OpenBao work
// end to end against the emulator-component path AND that the OpenBao state persists:
// `atmos emulator up openbao` brings up a file-backed OpenBao server (the container
// atmos manages, not a pre-started service) that the manager auto-initializes and
// unseals, `atmos secret set` writes a declared secret into its KV v2 engine through
// the hashicorp-vault store, and a consumer resolves it with `!secret` at apply time.
// The secret then survives `down`/`up` (persistence) and is gone after `reset`.
//
// Opt-in and runtime-gated like the AWS emulator lifecycle: it runs only when
// ATMOS_TEST_FLOCI=true and a container runtime (Docker or Podman) is available;
// otherwise it skips with a clear reason.
func TestEmulatorOpenBaoLifecycle(t *testing.T) {
	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true to run the emulator E2E")
	}

	ctx := context.Background()
	if _, err := container.DetectRuntime(ctx); err != nil {
		t.Skipf("no container runtime available for the emulator E2E: %v", err)
	}

	RequireTerraformOrTofu(t)
	ensureAtmosRunner(t)

	workdir := copyFlociScenarioFixture(t, filepath.Join("fixtures", "scenarios", "emulator-openbao"))

	const stack = "local"
	// Keep persisted state hermetic so the test cleans up after itself and the bootstrap
	// (unseal key + dynamic root token) is readable from the host bind mount.
	cacheHome := t.TempDir()
	// The store reads the dynamic root token from VAULT_TOKEN; it is refreshed from the
	// emulator after every `up` by harvestOpenBaoToken below. Clear other VAULT_/BAO_
	// env so a misconfiguration can never reach a real server.
	env := map[string]string{
		"ATMOS_CLI_CONFIG_PATH": workdir,
		"TF_IN_AUTOMATION":      "1",
		"ATMOS_XDG_CACHE_HOME":  cacheHome,
		"VAULT_ADDR":            "",
		"BAO_ADDR":              "",
		"BAO_TOKEN":             "",
	}

	run := func(timeout time.Duration, args ...string) (string, string, error) {
		return runFlociAtmosInDir(t, workdir, env, timeout, args...)
	}

	// The bootstrap file lands on the host through the persistence bind mount.
	bootstrapPath := filepath.Join(cacheHome, "atmos", "emulator",
		container.RuntimeName(stack, "emulator", "openbao"), "atmos-bootstrap.json")

	up := func() {
		_, stderr, err := run(3*time.Minute, "emulator", "up", "openbao", "-s", stack)
		require.NoErrorf(t, err, "emulator up failed: %s", stderr)
		waitOpenBaoHealthy(t)
		// Refresh VAULT_TOKEN from the (possibly re-initialized) server.
		env["VAULT_TOKEN"] = harvestOpenBaoToken(t, bootstrapPath)
	}

	up()
	t.Cleanup(func() {
		// Use `reset` (not `down`) so the persisted state is wiped from inside the
		// running container. The test ends in an `up` state, and on a rootful runtime
		// the container writes root-owned files into the t.TempDir()-backed cache that
		// Go's TempDir cleanup (running as the test user) otherwise cannot remove.
		_, _, _ = run(2*time.Minute, "emulator", "reset", "openbao", "-s", stack, "--force")
	})

	// Write the declared secret into the emulated OpenBao KV v2 store.
	_, stderr, err := run(2*time.Minute, "secret", "set", "APP_SECRET=s3cr3t-value", "-s", stack, "-c", "consumer")
	require.NoErrorf(t, err, "secret set failed: %s", stderr)

	// Apply the consumer: Atmos resolves `!secret APP_SECRET` from OpenBao and delivers it
	// off-disk as TF_VAR_app_secret. The non-sensitive length output proves the exact value
	// round-tripped ("s3cr3t-value" is 12 characters).
	requireConsumerSecretLen := func() {
		t.Helper()
		_, stderr, err := run(5*time.Minute, "terraform", "apply", "consumer", "-s", stack, "-auto-approve")
		require.NoErrorf(t, err, "apply consumer failed: %s", stderr)
		stdout, stderr, err := run(2*time.Minute, "terraform", "output", "consumer", "-s", stack)
		require.NoErrorf(t, err, "output consumer failed: %s", stderr)
		require.Contains(t, stdout, "app_secret_len", "consumer should emit the resolved-secret length output")
		require.Contains(t, stdout, "12", "resolved secret should be the 12-char value set via OpenBao")
	}
	requireConsumerSecretLen()

	// Persistence: stop and restart the emulator WITHOUT re-setting the secret. Because the
	// file-backed OpenBao state persists across down/up, the secret is still resolvable.
	_, stderr, err = run(2*time.Minute, "emulator", "down", "openbao", "-s", stack)
	require.NoErrorf(t, err, "emulator down failed: %s", stderr)
	up()
	requireConsumerSecretLen()

	// Reset: wipe the persisted state, restart, and confirm the secret is gone — resolving
	// `!secret APP_SECRET` against an empty store must now fail.
	_, stderr, err = run(2*time.Minute, "emulator", "reset", "openbao", "-s", stack, "--force")
	require.NoErrorf(t, err, "emulator reset failed: %s", stderr)
	up()
	_, _, err = run(5*time.Minute, "terraform", "apply", "consumer", "-s", stack, "-auto-approve")
	require.Error(t, err, "apply must fail after reset because the secret was wiped")
}

// waitOpenBaoHealthy blocks until the OpenBao API answers 200 at /v1/sys/health
// (initialized + unsealed) over the pinned host port.
func waitOpenBaoHealthy(t *testing.T) {
	t.Helper()
	healthURL := "http://localhost:" + openBaoHostPort + "/v1/sys/health"
	require.Eventually(t, func() bool {
		probeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, reqErr := http.NewRequestWithContext(probeCtx, http.MethodGet, healthURL, nil)
		if reqErr != nil {
			return false
		}
		resp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 90*time.Second, 2*time.Second, "OpenBao server did not become ready")
}

// harvestOpenBaoToken reads the dynamic root token the file-backed server generated
// at init, from the bootstrap file persisted to the host via the bind mount.
func harvestOpenBaoToken(t *testing.T, bootstrapPath string) string {
	t.Helper()
	var token string
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(bootstrapPath)
		if err != nil {
			return false
		}
		var boot struct {
			RootToken string `json:"root_token"`
		}
		if json.Unmarshal(data, &boot) != nil || boot.RootToken == "" {
			return false
		}
		token = boot.RootToken
		return true
	}, 30*time.Second, time.Second, "OpenBao bootstrap (root token) did not appear at %s", bootstrapPath)
	return token
}
