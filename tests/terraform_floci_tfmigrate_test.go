//nolint:depguard // The Floci harness creates AWS clients for opt-in AWS-compatible integration tests.
package tests

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

// TestTerraformFlociTfmigrateS3History verifies that tfmigrate history storage
// automatically inherits the component's S3 backend on an emulator: the
// fixture has NO .tfmigrate.hcl, so Atmos generates a default config from the
// backend section (bucket + endpoints.s3) that stores history in the same
// bucket under the namespaced key. The full lifecycle runs: seed legacy
// state, apply the refactored component (the tfmigrate hook moves the state
// and records history in the emulated bucket), then rerun to confirm
// history-mode idempotency.
func TestTerraformFlociTfmigrateS3History(t *testing.T) {
	endpoint := requireFlociEndpoint(t, "", "")
	// No terraform/tofu precondition: the fixture declares opentofu and
	// tfmigrate in dependencies.tools, so the Atmos toolchain installs both
	// (the floci-go CI job does not provide a terraform binary).
	ensureFlociAtmosRunner(t)

	workdir := copyFlociScenarioFixture(t, filepath.Join("fixtures", "scenarios", "terraform-floci-tfmigrate"))
	testID := uniqueFlociTestID(t)
	bucket := "tfmigrate-history-" + testID
	env := flociHarnessCommandEnv(t, endpoint, workdir, testID, false)
	env["ATMOS_TEST_TFMIGRATE_BUCKET"] = bucket

	client := newFlociS3Client(t, endpoint)
	createFlociS3Bucket(t, client, bucket)

	// Seed the legacy state: random_pet.legacy lands in the emulated S3 backend.
	_, stderr, err := runFlociAtmosInDir(
		t, workdir, env, 10*time.Minute,
		"terraform", "apply", "service-legacy", "-s", "test", "-auto-approve", "-i", "false",
	)
	require.NoError(t, err, stderr)

	// Apply the refactored component. The before.terraform.apply hook runs
	// `tfmigrate apply`, which moves random_pet.legacy to random_pet.service in
	// the S3 state and records the migration in S3 history storage.
	_, stderr, err = runFlociAtmosInDir(
		t, workdir, env, 10*time.Minute,
		"terraform", "apply", "service", "-s", "test", "-auto-approve", "-i", "false",
	)
	require.NoError(t, err, stderr)

	// The history object exists in the emulated bucket under the namespaced key
	// derived from the inherited backend, and records the applied migration.
	history := getFlociS3Object(t, client, bucket, "tfmigrate/test/service/default/history.json")
	require.Contains(t, history, "rename_legacy_pet")

	// The migrated state kept the resource: the S3 state object now holds
	// random_pet.service and no random_pet.legacy.
	state := getFlociS3Object(t, client, bucket, "svc/terraform.tfstate")
	require.Contains(t, state, `"service"`)
	require.NotContains(t, state, `"legacy"`)

	// Rerun the plan: history mode sees the applied migration and the run
	// stays clean (exit code asserts idempotency).
	_, stderr, err = runFlociAtmosInDir(
		t, workdir, env, 10*time.Minute,
		"terraform", "plan", "service", "-s", "test", "-i", "false",
	)
	require.NoError(t, err, stderr)
}

func newFlociS3Client(t *testing.T, endpoint string) *s3.Client {
	t.Helper()

	cfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(flociAWSRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
}

func createFlociS3Bucket(t *testing.T, client *s3.Client, bucket string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
}

func getFlociS3Object(t *testing.T, client *s3.Client, bucket, key string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	require.NoError(t, err, "expected s3://%s/%s to exist", bucket, key)
	defer out.Body.Close()

	body, err := io.ReadAll(out.Body)
	require.NoError(t, err)
	return string(body)
}
