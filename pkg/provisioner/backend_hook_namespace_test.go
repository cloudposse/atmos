package provisioner

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
)

// sdkBucketNamespace is a namespace value defined by the AWS SDK; the provisioner accepts it.
const sdkBucketNamespace = "account-regional"

// fakeS3Backend records how the auto-provisioning hook drives the registered S3 backend.
type fakeS3Backend struct {
	bucketExists bool
	existsCalls  int
	createCalls  int
	gotNamespace string
}

// register installs the fake as the S3 create and exists functions for the duration of the test.
func (f *fakeS3Backend) register(t *testing.T) {
	t.Helper()

	backend.ResetRegistryForTesting()
	t.Cleanup(backend.ResetRegistryForTesting)

	backend.RegisterBackendExists("s3", func(context.Context, *schema.AtmosConfiguration, map[string]any, *schema.AuthContext) (bool, error) {
		f.existsCalls++
		return f.bucketExists, nil
	})
	backend.RegisterBackendCreate("s3", func(_ context.Context, _ *schema.AtmosConfiguration, _ map[string]any, _ *schema.AuthContext, opts ...backend.CreateOption) (*backend.ProvisionResult, error) {
		f.createCalls++
		f.gotNamespace = backend.BucketNamespaceForTesting(opts...)
		return &backend.ProvisionResult{}, nil
	})
}

// runAutoProvision runs the terraform init auto-provisioning hook against the fake backend.
func runAutoProvision(t *testing.T, fake *fakeS3Backend, provisionBackend map[string]any) error {
	t.Helper()

	fake.register(t)

	componentConfig := map[string]any{
		"backend_type": "s3",
		"backend":      map[string]any{"bucket": "state-bucket", "region": "us-west-2"},
		"provision":    map[string]any{"backend": provisionBackend},
	}

	ctx := WithOutputSuppressed(context.Background())
	writers := OutputWriters{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}

	return autoProvisionBackend(ctx, &schema.AtmosConfiguration{}, componentConfig, nil, writers, nil)
}

func TestAutoProvisionBackend_BucketNamespace(t *testing.T) {
	t.Run("configured namespace reaches the create function", func(t *testing.T) {
		fake := &fakeS3Backend{}

		err := runAutoProvision(t, fake, map[string]any{"enabled": true, "bucket_namespace": sdkBucketNamespace})
		require.NoError(t, err)

		assert.Equal(t, 1, fake.createCalls)
		assert.Equal(t, sdkBucketNamespace, fake.gotNamespace)
	})

	t.Run("unset namespace passes no option", func(t *testing.T) {
		fake := &fakeS3Backend{}

		err := runAutoProvision(t, fake, map[string]any{"enabled": true})
		require.NoError(t, err)

		assert.Equal(t, 1, fake.createCalls)
		assert.Empty(t, fake.gotNamespace)
	})

	t.Run("unsupported namespace fails before the existence check", func(t *testing.T) {
		// The bucket exists, so without early validation the hook would return silently.
		fake := &fakeS3Backend{bucketExists: true}

		err := runAutoProvision(t, fake, map[string]any{"enabled": true, "bucket_namespace": "opaque-value"})
		require.ErrorIs(t, err, errUtils.ErrUnsupportedBucketNamespace)

		assert.Zero(t, fake.existsCalls, "the existence check must not run for an unsupported namespace")
		assert.Zero(t, fake.createCalls)
	})

	t.Run("existing bucket with a valid namespace is left alone", func(t *testing.T) {
		fake := &fakeS3Backend{bucketExists: true}

		err := runAutoProvision(t, fake, map[string]any{"enabled": true, "bucket_namespace": sdkBucketNamespace})
		require.NoError(t, err)

		assert.Equal(t, 1, fake.existsCalls)
		assert.Zero(t, fake.createCalls)
	})
}
