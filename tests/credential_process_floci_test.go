//nolint:depguard // The credential_process E2E tests create AWS SDK clients against the opt-in Floci emulator.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Opt-in AWS integration tests (Floci) for the AWS credential_process support:
//   - Producer: `atmos aws credential-process` (and `atmos auth env --format=credential-process`)
//     feeds an AWS SDK through a shared-config `credential_process` line.
//   - Consumer: an `aws/credential-process` identity runs that helper and writes Atmos-managed AWS files.
//   - Recursion guard: a helper that calls back into Atmos for the same identity fails fast.
//   - Chain: `aws/assume-role` chained from an `aws/credential-process` identity.

const (
	credentialProcessFixtureDir = "fixtures/scenarios/aws-credential-process-floci"

	// Environment variables the fixture reads for the credential_process commands.
	credentialProcessBaseCmdEnv = "ATMOS_TEST_CREDENTIAL_PROCESS_BASE_CMD"
	credentialProcessLoopCmdEnv = "ATMOS_TEST_CREDENTIAL_PROCESS_LOOP_CMD"

	credentialProcessTimeout = 3 * time.Minute
)

// processCredentialDocument is the JSON document defined by the AWS SDKs for `credential_process`.
type processCredentialDocument struct {
	Version         int    `json:"Version"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"SessionToken"`
	Expiration      string `json:"Expiration"`
}

// credentialProcessQuote double-quotes an executable path. Plain double quotes with no escaping are
// valid, unambiguous syntax in both POSIX sh and Windows cmd.exe for a path without embedded quotes.
func credentialProcessQuote(path string) string {
	return `"` + path + `"`
}

// credentialProcessHelperCommand returns the credential_process command that asks the atmos binary
// under test for the identity's credentials.
func credentialProcessHelperCommand(identity string) string {
	return credentialProcessQuote(atmosRunner.BinaryPath()) + " aws credential-process --identity=" + identity
}

// newCredentialProcessHarness creates a Floci harness for the credential_process fixture with an
// isolated keyring and XDG config directory, and the credential_process commands injected.
func newCredentialProcessHarness(t *testing.T) *flociHarness {
	t.Helper()

	harness := newFlociHarness(t, flociHarnessOptions{
		FixtureDir:  credentialProcessFixtureDir,
		ClearAWSEnv: true,
	})

	xdgConfigHome := t.TempDir()
	harness.Env["ATMOS_KEYRING_TYPE"] = "memory"
	harness.Env["ATMOS_XDG_CONFIG_HOME"] = xdgConfigHome
	harness.Env["XDG_CONFIG_HOME"] = xdgConfigHome
	harness.Env["XDG_CACHE_HOME"] = filepath.Join(t.TempDir(), ".cache")
	harness.Env[credentialProcessBaseCmdEnv] = credentialProcessHelperCommand("base")
	harness.Env[credentialProcessLoopCmdEnv] = credentialProcessHelperCommand("loop")
	return harness
}

// applyCredentialProcessEnv exports the harness environment to the test process so helper commands
// spawned by the AWS SDK (which inherit os.Environ) see the same Atmos configuration.
func applyCredentialProcessEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for key, value := range env {
		t.Setenv(key, value)
	}
}

// parseProcessCredentialDocument parses and validates the stdout of a credential_process helper.
func parseProcessCredentialDocument(t *testing.T, stdout string) processCredentialDocument {
	t.Helper()

	require.True(t, strings.HasSuffix(stdout, "\n"), "document must be followed by a newline: %q", stdout)
	var doc processCredentialDocument
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), "stdout must be a single JSON document")

	assert.Equal(t, 1, doc.Version)
	assert.NotEmpty(t, doc.AccessKeyID)
	assert.NotEmpty(t, doc.SecretAccessKey)
	assert.NotEmpty(t, doc.SessionToken, "session credentials must carry a session token")
	expiration, err := time.Parse(time.RFC3339, doc.Expiration)
	require.NoError(t, err, "Expiration must be RFC3339: %q", doc.Expiration)
	assert.True(t, expiration.After(time.Now()), "Expiration must be in the future: %s", doc.Expiration)
	return doc
}

// newProcessCredentialSDKConfig loads an AWS SDK config from the given shared config/credentials
// files and profile only (no ambient credentials).
func newProcessCredentialSDKConfig(t *testing.T, configFile, credentialsFile, profile string) aws.Config {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), credentialProcessTimeout)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithSharedConfigFiles([]string{configFile}),
		awsconfig.WithSharedCredentialsFiles([]string{credentialsFile}),
		awsconfig.WithSharedConfigProfile(profile),
		awsconfig.WithRegion(flociAWSRegion),
	)
	require.NoError(t, err)
	return cfg
}

// requireFlociCallerIdentityAndS3 proves the SDK config carries working credentials: STS
// GetCallerIdentity succeeds and an S3 bucket can be created, listed, and deleted.
func requireFlociCallerIdentityAndS3(t *testing.T, cfg *aws.Config, endpoint, testID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), credentialProcessTimeout)
	defer cancel()

	stsClient := sts.NewFromConfig(*cfg, func(o *sts.Options) { o.BaseEndpoint = aws.String(endpoint) })
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	require.NoError(t, err, "sts:GetCallerIdentity with credential_process credentials failed")
	require.NotNil(t, identity.Account)
	assert.NotEmpty(t, aws.ToString(identity.Account))

	s3Client := s3.NewFromConfig(*cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	bucket := "atmos-credproc-" + strings.ReplaceAll(testID, "_", "-")
	if len(bucket) > 63 {
		bucket = strings.TrimRight(bucket[:63], "-")
	}
	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err, "s3:CreateBucket failed")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = s3Client.DeleteBucket(cleanupCtx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	buckets, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	require.NoError(t, err, "s3:ListBuckets failed")
	names := make([]string, 0, len(buckets.Buckets))
	for _, b := range buckets.Buckets {
		names = append(names, aws.ToString(b.Name))
	}
	assert.Contains(t, names, bucket)
}

// writeProcessCredentialSharedConfig writes an AWS shared config file with a profile whose
// credentials come from the given credential_process command, plus an empty credentials file.
func writeProcessCredentialSharedConfig(t *testing.T, profile, command string) (configFile, credentialsFile string) {
	t.Helper()

	dir := t.TempDir()
	configFile = filepath.Join(dir, "config")
	credentialsFile = filepath.Join(dir, "credentials")

	content := "[profile " + profile + "]\n" +
		"credential_process = " + command + "\n" +
		"region = " + flociAWSRegion + "\n"
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o600))
	require.NoError(t, os.WriteFile(credentialsFile, nil, 0o600))
	return configFile, credentialsFile
}

func TestAWSCredentialProcessFlociE2E_Producer(t *testing.T) {
	harness := newCredentialProcessHarness(t)

	t.Run("aws credential-process prints a process-credential document", func(t *testing.T) {
		stdout, stderr, err := harness.Run(t, credentialProcessTimeout, "aws", "credential-process", "--identity=base")
		require.NoError(t, err, "atmos aws credential-process failed:\n%s", stderr)
		parseProcessCredentialDocument(t, stdout)
	})

	t.Run("auth env --format=credential-process prints the same document shape", func(t *testing.T) {
		stdout, stderr, err := harness.Run(t, credentialProcessTimeout, "auth", "env", "--identity=base", "--format=credential-process")
		require.NoError(t, err, "atmos auth env --format=credential-process failed:\n%s", stderr)
		parseProcessCredentialDocument(t, stdout)
	})

	t.Run("AWS SDK sources credentials from the helper", func(t *testing.T) {
		applyCredentialProcessEnv(t, harness.Env)

		configFile, credentialsFile := writeProcessCredentialSharedConfig(t, "atmos-base", credentialProcessHelperCommand("base"))
		cfg := newProcessCredentialSDKConfig(t, configFile, credentialsFile, "atmos-base")

		// Prove the SDK resolved credentials through credential_process before using them.
		ctx, cancel := context.WithTimeout(context.Background(), credentialProcessTimeout)
		defer cancel()
		creds, err := cfg.Credentials.Retrieve(ctx)
		require.NoError(t, err, "credential_process helper did not produce credentials for the SDK")
		assert.NotEmpty(t, creds.AccessKeyID)
		assert.NotEmpty(t, creds.SessionToken)
		assert.True(t, creds.CanExpire, "helper credentials must carry an expiration")

		requireFlociCallerIdentityAndS3(t, &cfg, harness.Endpoint, harness.TestID)
	})
}

func TestAWSCredentialProcessFlociE2E_Consumer(t *testing.T) {
	harness := newCredentialProcessHarness(t)

	// auth env --login authenticates the aws/credential-process identity (running the helper) and
	// then prints the environment pointing at the Atmos-managed AWS files.
	stdout, stderr, err := harness.Run(t, credentialProcessTimeout, "auth", "env", "--identity=piped", "--login", "--format=json")
	require.NoError(t, err, "atmos auth env --identity=piped failed:\n%s", stderr)

	var env map[string]string
	require.NoError(t, json.Unmarshal([]byte(stdout), &env), "auth env --format=json output: %s", stdout)

	credentialsFile := env["AWS_SHARED_CREDENTIALS_FILE"]
	configFile := env["AWS_CONFIG_FILE"]
	profile := env["AWS_PROFILE"]
	require.NotEmpty(t, credentialsFile, "AWS_SHARED_CREDENTIALS_FILE missing from: %s", stdout)
	require.NotEmpty(t, configFile, "AWS_CONFIG_FILE missing from: %s", stdout)
	require.NotEmpty(t, profile, "AWS_PROFILE missing from: %s", stdout)

	// The Atmos-managed files live under the synthetic aws-credential-process provider directory.
	assert.Contains(t, filepath.ToSlash(credentialsFile), "aws-credential-process")
	assert.Contains(t, filepath.ToSlash(configFile), "aws-credential-process")
	require.FileExists(t, credentialsFile)

	cfg := newProcessCredentialSDKConfig(t, configFile, credentialsFile, profile)
	requireFlociCallerIdentityAndS3(t, &cfg, harness.Endpoint, harness.TestID)

	// A second invocation reuses the cached files instead of failing or re-running the helper in a way
	// that invalidates them.
	_, stderr, err = harness.Run(t, credentialProcessTimeout, "auth", "env", "--identity=piped", "--login", "--format=json")
	require.NoError(t, err, "second atmos auth env --identity=piped failed:\n%s", stderr)
}

func TestAWSCredentialProcessFlociE2E_RecursionGuard(t *testing.T) {
	harness := newCredentialProcessHarness(t)

	// The helper for identity "loop" asks Atmos for credentials for "loop" again, so the second
	// level must fail fast instead of recursing forever.
	start := time.Now()
	stdout, stderr, err := harness.Run(t, 2*time.Minute, "auth", "env", "--identity=loop", "--format=credential-process")
	elapsed := time.Since(start)

	require.Error(t, err, "recursive credential_process must fail; stdout:\n%s", stdout)
	assert.NotErrorIs(t, err, context.DeadlineExceeded, "recursion must be detected, not time out")
	assert.Contains(t, strings.ToLower(stderr), "recursion", "stderr:\n%s", stderr)
	assert.Less(t, elapsed, 90*time.Second, "recursion guard must terminate quickly")
	assert.NotContains(t, stdout, "AccessKeyId", "no credentials may be printed on failure")
}

func TestAWSCredentialProcessFlociE2E_AssumeRoleChain(t *testing.T) {
	harness := newCredentialProcessHarness(t)

	stdout, stderr, err := harness.Run(t, credentialProcessTimeout, "aws", "credential-process", "--identity=chained")
	if err != nil {
		lower := strings.ToLower(stderr)
		if strings.Contains(lower, "not implemented") || strings.Contains(lower, "unsupported") || strings.Contains(lower, "not supported") {
			t.Skipf("Floci does not support sts:AssumeRole in this configuration:\n%s", stderr)
		}
		require.NoError(t, err, "atmos aws credential-process --identity=chained failed:\n%s", stderr)
	}

	doc := parseProcessCredentialDocument(t, stdout)

	// The chained identity must hand out role session credentials, not the source helper's.
	baseOut, baseStderr, err := harness.Run(t, credentialProcessTimeout, "aws", "credential-process", "--identity=piped")
	if err == nil {
		// Only meaningful when aws/credential-process identities can be requested directly.
		base := parseProcessCredentialDocument(t, baseOut)
		assert.NotEqual(t, base.AccessKeyID, doc.AccessKeyID, "assumed-role credentials must differ from the source credentials")
	} else {
		t.Logf("source identity not requested directly: %s", baseStderr)
	}
}
