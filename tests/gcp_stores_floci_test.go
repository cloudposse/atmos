//nolint:forbidigo // This opt-in integration test is configured through environment variables.
package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGCPSecretsFlociE2E(t *testing.T) {
	harness := newFlociHarness(t, flociHarnessOptions{
		FixtureDir:      "fixtures/scenarios/gcp-secrets-floci",
		EndpointEnvVar:  "FLOCI_GCP_ENDPOINT",
		DefaultEndpoint: "http://localhost:4588",
		ExtraEnv: map[string]string{
			"FLOCI_GCP_PROJECT_ID":     "floci-local",
			"FLOCI_GCP_SECRETS_PREFIX": "atmos-tests-gcp-{test_id}",
		},
	})

	_, stderr, err := harness.Run(t, 2*time.Minute, "validate", "stacks")
	require.NoError(t, err, "validate stacks failed:\n%s", stderr)

	_, stderr, err = harness.Run(
		t, 2*time.Minute,
		"secret", "set", "GCP_INSTANCE_TOKEN=gcp-instance-token", "-s", "dev", "-c", "secret-consumer", "--force",
	)
	require.NoError(t, err, "secret set failed:\n%s", stderr)

	t.Cleanup(func() {
		_, _, _ = harness.Run(
			t, 2*time.Minute,
			"secret", "delete", "GCP_INSTANCE_TOKEN", "-s", "dev", "-c", "secret-consumer", "--force",
		)
	})

	valueOut, stderr, err := harness.Run(
		t, 2*time.Minute,
		"secret", "get", "GCP_INSTANCE_TOKEN", "-s", "dev", "-c", "secret-consumer", "--mask=false",
	)
	require.NoError(t, err, "secret get failed:\n%s", stderr)
	require.Contains(t, valueOut, "gcp-instance-token")

	listOut, stderr, err := harness.Run(t, 2*time.Minute, "secret", "list", "-s", "dev", "-c", "secret-consumer")
	require.NoError(t, err, "secret list failed:\n%s", stderr)
	require.Contains(t, listOut, "GCP_INSTANCE_TOKEN")

	_, stderr, err = harness.Run(t, 2*time.Minute, "secret", "validate", "-s", "dev", "-c", "secret-consumer")
	require.NoError(t, err, "secret validate failed:\n%s", stderr)

	maskedDescribe, stderr, err := harness.Run(
		t, 2*time.Minute,
		"describe", "component", "secret-consumer", "-s", "dev", "--format", "json",
	)
	require.NoError(t, err, "masked describe failed:\n%s", stderr)
	require.Contains(t, maskedDescribe, "<MASKED>")
	require.NotContains(t, maskedDescribe, "gcp-instance-token")
}
