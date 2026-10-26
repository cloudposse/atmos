//nolint:depguard // This opt-in AWS integration test verifies SOPS aws-kms against a Floci KMS emulator.
package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/require"
)

// TestSopsKmsFlociE2E reproduces issue #2637: `atmos secret` against a SOPS aws-kms backend must
// authenticate using the Atmos identity (here `floci-superuser`) and use ITS credentials for the
// KMS encrypt/decrypt — WITHOUT any ambient AWS credentials in the process environment.
//
// The harness clears all ambient AWS credentials (ClearAWSEnv). The only credentials available are
// the ones the `floci-superuser` identity carries in atmos.yaml. The Floci KMS endpoint is supplied
// via AWS_ENDPOINT_URL/AWS_ENDPOINT_URL_KMS (endpoint is infrastructure, not a credential).
//
// Before the fix this fails because the SOPS provider ignores the identity and falls back to the
// ambient credential chain (which is empty), producing the classic
// "Error getting data key: 0 successful groups required" failure.
func TestSopsKmsFlociE2E(t *testing.T) {
	endpoint := os.Getenv("FLOCI_ENDPOINT_URL")
	if endpoint == "" {
		endpoint = "http://localhost:4566"
	}

	harness := newFlociHarness(t, flociHarnessOptions{
		FixtureDir:  "fixtures/scenarios/sops-kms-floci",
		ClearAWSEnv: true,
		ExtraEnv: map[string]string{
			// Point the in-process getsops KMS client at Floci. This is endpoint configuration,
			// not credentials — the credentials must still come from the Atmos identity.
			"AWS_ENDPOINT_URL":     endpoint,
			"AWS_ENDPOINT_URL_KMS": endpoint,
		},
	})

	// Create a KMS key in Floci and write a .sops.yaml creation rule referencing its ARN.
	keyArn := createFlociKMSKey(t, harness.Endpoint)
	writeSopsCreationRule(t, harness.Workdir, keyArn)

	// secret set encrypts the data key with KMS — requires identity credentials.
	_, stderr, err := harness.Run(
		t, 2*time.Minute,
		"secret", "set", "API_KEY=supersecret-value", "-s", "dev", "-c", "app", "--force",
		"--identity", "floci-superuser",
	)
	require.NoError(t, err, "secret set (encrypt via aws-kms identity) failed:\n%s", stderr)

	// secret get decrypts the data key with KMS — requires identity credentials.
	stdout, stderr, err := harness.Run(
		t, 2*time.Minute,
		"secret", "get", "API_KEY", "-s", "dev", "-c", "app", "--mask=false",
		"--identity", "floci-superuser",
	)
	require.NoError(t, err, "secret get (decrypt via aws-kms identity) failed:\n%s", stderr)
	require.Contains(t, stdout, "supersecret-value")
}

// createFlociKMSKey creates a symmetric KMS key in the Floci emulator and returns its ARN.
func createFlociKMSKey(t *testing.T, endpoint string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(flociAWSRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	client := kms.NewFromConfig(cfg, func(o *kms.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	out, err := client.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err, "creating Floci KMS key")
	require.NotNil(t, out.KeyMetadata)
	require.NotNil(t, out.KeyMetadata.Arn)
	return *out.KeyMetadata.Arn
}

// writeSopsCreationRule writes a .sops.yaml in the workdir whose creation rule encrypts the
// secrets/*.enc.yaml files with the given KMS ARN.
func writeSopsCreationRule(t *testing.T, workdir, keyArn string) {
	t.Helper()

	content := fmt.Sprintf("creation_rules:\n  - path_regex: secrets/.*\\.enc\\.yaml$\n    kms: %s\n", keyArn)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, ".sops.yaml"), []byte(content), 0o600))
}
