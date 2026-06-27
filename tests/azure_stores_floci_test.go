package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAzureSecretsFlociE2E(t *testing.T) {
	harness := newFlociHarness(t, flociHarnessOptions{
		FixtureDir:      "fixtures/scenarios/azure-secrets-floci",
		EndpointEnvVar:  "FLOCI_AZURE_ENDPOINT",
		DefaultEndpoint: "http://localhost:4577",
		ExtraEnv: map[string]string{
			"FLOCI_AZURE_SECRETS_PREFIX": "atmos-tests-azure-{test_id}",
		},
	})
	harness.Env["FLOCI_AZURE_KEYVAULT_ENDPOINT"] = strings.TrimRight(harness.Endpoint, "/") + "/devstoreaccount1-keyvault"

	_, stderr, err := harness.Run(t, 2*time.Minute, "validate", "stacks")
	require.NoError(t, err, "validate stacks failed:\n%s", stderr)

	_, stderr, err = harness.Run(
		t, 2*time.Minute,
		"secret", "set", "AZURE_INSTANCE_TOKEN=azure-instance-token", "-s", "dev", "-c", "secret-consumer", "--force",
	)
	require.NoError(t, err, "secret set failed:\n%s", stderr)

	t.Cleanup(func() {
		_, _, _ = harness.Run(
			t, 2*time.Minute,
			"secret", "delete", "AZURE_INSTANCE_TOKEN", "-s", "dev", "-c", "secret-consumer", "--force",
		)
	})

	valueOut, stderr, err := harness.Run(
		t, 2*time.Minute,
		"secret", "get", "AZURE_INSTANCE_TOKEN", "-s", "dev", "-c", "secret-consumer", "--mask=false",
	)
	require.NoError(t, err, "secret get failed:\n%s", stderr)
	require.Contains(t, valueOut, "azure-instance-token")

	listOut, stderr, err := harness.Run(t, 2*time.Minute, "secret", "list", "-s", "dev", "-c", "secret-consumer")
	require.NoError(t, err, "secret list failed:\n%s", stderr)
	require.Contains(t, listOut, "AZURE_INSTANCE_TOKEN")

	_, stderr, err = harness.Run(t, 2*time.Minute, "secret", "validate", "-s", "dev", "-c", "secret-consumer")
	require.NoError(t, err, "secret validate failed:\n%s", stderr)

	maskedDescribe, stderr, err := harness.Run(
		t, 2*time.Minute,
		"describe", "component", "secret-consumer", "-s", "dev", "--format", "json",
	)
	require.NoError(t, err, "masked describe failed:\n%s", stderr)
	require.Contains(t, maskedDescribe, "<MASKED>")
	require.NotContains(t, maskedDescribe, "azure-instance-token")
}
