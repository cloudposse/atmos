package s3test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ambientAWSVariables are the variables through which the developer's or the runner's AWS setup
// reaches the SDK's default chain: the profile selection, the web identity and container credential
// sources, and the endpoint, FIPS, and dual-stack overrides. Static credentials and the region are
// left to each test, which sets the ones it needs.
var ambientAWSVariables = []string{
	"AWS_PROFILE",
	"AWS_DEFAULT_PROFILE",
	"AWS_SESSION_TOKEN",
	"AWS_ROLE_ARN",
	"AWS_ROLE_SESSION_NAME",
	"AWS_WEB_IDENTITY_TOKEN_FILE",
	"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
	"AWS_CONTAINER_CREDENTIALS_FULL_URI",
	"AWS_CONTAINER_AUTHORIZATION_TOKEN",
	"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE",
	"AWS_ENDPOINT_URL",
	"AWS_ENDPOINT_URL_S3",
	"AWS_USE_FIPS_ENDPOINT",
	"AWS_USE_DUALSTACK_ENDPOINT",
}

// isolatedFileMode is the permission of the empty shared files, which hold no secret.
const isolatedFileMode = 0o600

// IsolateAWSConfig keeps a test that builds an AWS SDK configuration away from the AWS setup of the
// machine it runs on. The shared config and credentials files point at empty temporary files, the
// variables that select a profile or another credential source are cleared, and instance metadata is
// disabled. A developer's default profile with a role_arn would otherwise be merged into the test's
// configuration and select AssumeRole, which reaches the real STS service. It returns the directory
// holding the files, so a test can write the profile it wants into them.
func IsolateAWSConfig(t testing.TB) string {
	defer perf.Track(nil, "s3test.IsolateAWSConfig")()

	t.Helper()
	dir := t.TempDir()
	config, credentials := filepath.Join(dir, "config"), filepath.Join(dir, "credentials")
	for _, path := range []string{config, credentials} {
		if err := os.WriteFile(path, nil, isolatedFileMode); err != nil {
			t.Fatalf("create an empty AWS file: %v", err)
		}
	}
	for _, name := range ambientAWSVariables {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_CONFIG_FILE", config)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentials)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	return dir
}
