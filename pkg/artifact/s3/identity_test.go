package s3

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// isolateAWS gives the SDK static environment credentials and no ambient profile, so a test proves
// what the repository does with an identity rather than what the developer machine happens to have.
func isolateAWS(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AWS_ACCESS_KEY_ID", "ambient-test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-test-secret")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// resolverStub records calls and returns a canned value.
type resolverStub struct {
	calls  atomic.Int32
	values []any
	errs   []error
}

func (r *resolverStub) resolve(_ context.Context, _ string) (any, error) {
	index := int(r.calls.Add(1)) - 1
	if index >= len(r.values) {
		index = len(r.values) - 1
	}
	return r.values[index], r.errs[index]
}

func identityRepo(t *testing.T, fake *s3test.Server, resolver artifact.IdentityResolver, identity string) *Repository {
	t.Helper()
	cfg := &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindS3, Identity: identity,
		Options: map[string]any{"bucket": fake.Bucket(), "endpoint": fake.Endpoint(), "region": "us-east-1"},
	}
	var ropts []artifact.RepositoryOption
	if resolver != nil {
		ropts = append(ropts, artifact.WithIdentityResolver(resolver))
	}
	applied := artifact.ApplyRepositoryOptions(ropts...)
	repo, err := newRepository("shared", cfg, nil, &applied)
	require.NoError(t, err)
	return repo
}

// Constructing, describing, and asking for capabilities resolve no identity and touch no endpoint.
func TestIdentity_IsResolvedLazily(t *testing.T) {
	isolateAWS(t)
	fake := s3test.NewServer(t)
	stub := &resolverStub{values: []any{&store.AWSAuthConfig{Region: "us-east-1"}}, errs: []error{nil}}
	repo := identityRepo(t, fake, stub.resolve, "deploy")
	_ = repo.Capabilities()
	_ = repo.Location()
	_ = Descriptor()
	assert.Zero(t, stub.calls.Load(), "no identity is resolved before an operation needs credentials")
	assert.Empty(t, fake.Requests())

	_, err := repo.List(context.Background(), artifact.ListQuery{})
	require.NoError(t, err)
	_, err = repo.List(context.Background(), artifact.ListQuery{})
	require.NoError(t, err)
	assert.Equal(t, int32(1), stub.calls.Load(), "the resolved client is reused")
	assert.NotEmpty(t, fake.Requests())
}

// An identity that cannot be resolved fails closed: no request is sent, whatever ambient
// credentials the environment offers.
func TestIdentity_FailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		resolver func() artifact.IdentityResolver
	}{
		{name: "no resolver injected", resolver: func() artifact.IdentityResolver { return nil }},
		{name: "resolver fails", resolver: func() artifact.IdentityResolver {
			return (&resolverStub{values: []any{nil}, errs: []error{errors.New("sso session expired")}}).resolve
		}},
		{name: "resolver returns nothing", resolver: func() artifact.IdentityResolver {
			return (&resolverStub{values: []any{nil}, errs: []error{nil}}).resolve
		}},
		{name: "resolver returns another cloud's credentials", resolver: func() artifact.IdentityResolver {
			return (&resolverStub{values: []any{"azure-token"}, errs: []error{nil}}).resolve
		}},
		{name: "resolver returns a nil AWS config", resolver: func() artifact.IdentityResolver {
			var none *store.AWSAuthConfig
			return (&resolverStub{values: []any{none}, errs: []error{nil}}).resolve
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateAWS(t)
			fake := s3test.NewServer(t)
			repo := identityRepo(t, fake, tt.resolver(), "deploy")

			for _, operation := range []func() error{
				func() error { _, err := repo.Resolve(context.Background(), defaultRef()); return err },
				func() error { _, err := repo.List(context.Background(), artifact.ListQuery{}); return err },
				func() error {
					_, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: "x"})
					return err
				},
			} {
				err := operation()
				require.ErrorIs(t, err, errUtils.ErrBlobIdentityUnresolved)
				assert.NotErrorIs(t, err, errUtils.ErrArtifactNotFound, "an unresolved identity is never reported as an empty repository")
			}
			assert.Empty(t, fake.Requests(), "no request reached the endpoint with ambient credentials")
		})
	}
}

// A transient identity failure is not cached: the next operation resolves again.
func TestIdentity_TransientFailureIsRetried(t *testing.T) {
	isolateAWS(t)
	fake := s3test.NewServer(t)
	stub := &resolverStub{
		values: []any{nil, &store.AWSAuthConfig{Region: "us-east-1"}},
		errs:   []error{errors.New("temporarily unavailable"), nil},
	}
	repo := identityRepo(t, fake, stub.resolve, "deploy")
	_, err := repo.List(context.Background(), artifact.ListQuery{})
	require.ErrorIs(t, err, errUtils.ErrBlobIdentityUnresolved)
	_, err = repo.List(context.Background(), artifact.ListQuery{})
	require.NoError(t, err)
	assert.Equal(t, int32(2), stub.calls.Load())
}

// Without an identity the default credential chain applies, as for any AWS SDK user.
func TestIdentity_AbsentUsesTheDefaultChain(t *testing.T) {
	isolateAWS(t)
	fake := s3test.NewServer(t)
	repo := identityRepo(t, fake, nil, "")
	_, err := repo.List(context.Background(), artifact.ListQuery{})
	require.NoError(t, err)
	assert.NotEmpty(t, fake.Requests())
}

// The factory path end to end against the fake: a conditional-write probe, a publish, and a read.
func TestFactoryRepository_PublishesThroughTheSDKClient(t *testing.T) {
	isolateAWS(t)
	fake := s3test.NewServer(t)
	for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTgz} {
		t.Run(string(packaging), func(t *testing.T) {
			cfg := &schema.ArtifactRepositoryConfig{
				Kind: artifact.RepositoryKindS3, Packaging: string(packaging),
				Options: map[string]any{
					"bucket": fake.Bucket(), "endpoint": fake.Endpoint(), "region": "us-east-1", "prefix": "factory/" + string(packaging),
					"tags": map[string]any{"team": "platform"},
				},
			}
			repo, err := newRepository("shared", cfg, nil, &artifact.RepositoryOptions{})
			require.NoError(t, err)
			result := publish(t, repo, &prepareSpec{files: bigFiles()}, artifact.PublishOptions{})
			assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
			assertIntact(t, repo, defaultRef())
			object, ok := fake.Object("factory/" + string(packaging) + "/.atmos-artifacts/v1/refs/" + refKeyOnly(testName, testVersion) + ".json")
			require.True(t, ok)
			assert.Equal(t, map[string]string{"team": "platform"}, object.Tags, "the production store tags what the repository writes")
		})
	}
	assert.NotEmpty(t, fake.RequestsFor(s3test.OpPutObject))
}
