package aws

// Tests that the identity credentials loader and awsCloud.WithIsolatedAWSEnv
// serialize their changes to the process environment.

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
)

// TestAWSProcessEnv_LoaderAndIsolationDoNotInterleave runs the credentials
// loader (which points AWS_PROFILE and the shared files at an identity) next to
// WithIsolatedAWSEnv (which must see none of them) and verifies that neither
// observes the other's environment and that the user's environment is restored.
func TestAWSProcessEnv_LoaderAndIsolationDoNotInterleave(t *testing.T) {
	dir := t.TempDir()
	credsFile := filepath.Join(dir, "credentials")
	configFile := filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(credsFile, []byte("[dev]\naws_access_key_id = AKIADEV\naws_secret_access_key = secret\n"), 0o600))
	require.NoError(t, os.WriteFile(configFile, []byte("[profile dev]\nregion = us-west-2\n"), 0o600))

	t.Setenv("AWS_PROFILE", "user-profile")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "user-credentials"))
	env := map[string]string{
		"AWS_SHARED_CREDENTIALS_FILE": credsFile,
		"AWS_CONFIG_FILE":             configFile,
		"AWS_PROFILE":                 "dev",
		"AWS_REGION":                  "us-west-2",
	}

	const iterations = 200
	var leaks atomic.Int32
	var loaderErrs atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range iterations {
			creds, err := loadAWSCredentialsFromEnvironment(context.Background(), env)
			if err != nil || creds.AccessKeyID != "AKIADEV" {
				loaderErrs.Add(1)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range iterations {
			_ = awsCloud.WithIsolatedAWSEnv(func() error {
				if os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_SHARED_CREDENTIALS_FILE") != "" {
					leaks.Add(1)
				}
				return nil
			})
		}
	}()
	wg.Wait()

	assert.Zero(t, leaks.Load(), "isolated callbacks saw the loader's AWS_PROFILE or credentials file")
	assert.Zero(t, loaderErrs.Load(), "the loader lost its environment to a concurrent isolation")
	assert.Equal(t, "user-profile", os.Getenv("AWS_PROFILE"), "the user's AWS_PROFILE must be restored")
	assert.Equal(t, filepath.Join(dir, "user-credentials"), os.Getenv("AWS_SHARED_CREDENTIALS_FILE"))
}
