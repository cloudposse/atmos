package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

// TestEmulatorNativeAWSLifecycle exercises the NATIVE emulator path end to end:
// `atmos emulator up` brings the container up itself (not a pre-started Floci),
// the aws/emulator identity + provider-config contributor let Terraform apply real
// resources against it with no providers.tf, cross-component data flows through both
// `!terraform.output` and store hooks + `!store`, and the secrets engine resolves a
// declared secret — all against the emulator with zero cloud credentials.
//
// Opt-in and runtime-gated: it runs only when ATMOS_TEST_FLOCI=true and a container
// runtime (Docker or Podman) is available; otherwise it skips with a clear reason.
// Assertions are control-plane only (resource provisioning + data flow), never the
// runtime data plane (Floci emulates the control plane only).
func TestEmulatorNativeAWSLifecycle(t *testing.T) {
	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true to run the native emulator E2E")
	}

	ctx := context.Background()
	if _, err := container.DetectRuntime(ctx); err != nil {
		t.Skipf("no container runtime available for the native emulator E2E: %v", err)
	}

	RequireTerraformOrTofu(t)
	ensureAtmosRunner(t)

	workdir := copyFlociScenarioFixture(t, filepath.Join("fixtures", "scenarios", "emulator-native-aws"))

	// Use the copied fixture as the config root and keep real AWS credentials out of
	// the environment so a misconfiguration can never reach a real account.
	env := map[string]string{
		"ATMOS_CLI_CONFIG_PATH": workdir,
		"TF_IN_AUTOMATION":      "1",
		"AWS_PROFILE":           "",
		"AWS_ACCESS_KEY_ID":     "",
		"AWS_SECRET_ACCESS_KEY": "",
		"AWS_SESSION_TOKEN":     "",
	}
	const stack = "local"

	run := func(timeout time.Duration, args ...string) (string, string, error) {
		return runFlociAtmosInDir(t, workdir, env, timeout, args...)
	}

	// Start the sandbox; always tear the container down (it holds all state).
	_, stderr, err := run(3*time.Minute, "emulator", "up", "aws", "-s", stack)
	require.NoErrorf(t, err, "emulator up failed: %s", stderr)
	t.Cleanup(func() {
		_, _, _ = run(2*time.Minute, "emulator", "down", "aws", "-s", stack)
	})

	// Deploy the producers in dependency order (KMS -> producer).
	for _, component := range []string{"kms", "producer"} {
		_, stderr, err := run(5*time.Minute, "terraform", "apply", component, "-s", stack, "-auto-approve")
		require.NoErrorf(t, err, "apply %s failed: %s", component, stderr)
	}

	// Provision the declared secret into the emulated SSM SecureString store.
	_, stderr, err = run(2*time.Minute, "secret", "set", "APP_SECRET=s3cr3t-value", "-s", stack, "-c", "consumer")
	require.NoErrorf(t, err, "secret set failed: %s", stderr)

	// `secret list --verify` confirms it initialized: the SSM store is remote, so
	// existence is only known by authenticating to the emulator and checking it
	// (without --verify, remote-store status is reported as "unknown" by design).
	stdout, stderr, err := run(2*time.Minute, "secret", "list", "-s", stack, "-c", "consumer", "--verify")
	require.NoErrorf(t, err, "secret list failed: %s", stderr)
	require.Contains(t, stdout+stderr, "initialized", "declared secret should be initialized after set")

	// Apply the consumer: this resolves `!store` (producer coordinate) and `!secret`
	// (the value just set) and applies against the emulator.
	_, stderr, err = run(5*time.Minute, "terraform", "apply", "consumer", "-s", stack, "-auto-approve")
	require.NoErrorf(t, err, "apply consumer failed: %s", stderr)

	// The consumer output echoes the producer bucket id received via `!store`,
	// proving the cross-component store flow resolved against the emulator.
	stdout, stderr, err = run(2*time.Minute, "terraform", "output", "consumer", "-s", stack)
	require.NoErrorf(t, err, "output consumer failed: %s", stderr)
	require.Contains(t, stdout, "emu-native-local-producer", "consumer should receive the producer bucket id via !store")

	// Best-effort destroy in reverse order; the deferred emulator-down is the backstop.
	for _, component := range []string{"consumer", "producer", "kms"} {
		_, _, _ = run(5*time.Minute, "terraform", "destroy", component, "-s", stack, "-auto-approve")
	}
}
