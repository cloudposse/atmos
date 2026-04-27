package s3

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestStore_Name(t *testing.T) {
	store := &Store{
		bucket: "test-bucket",
	}
	assert.Equal(t, "aws/s3", store.Name())
}

func TestStore_fullKey(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		key      string
		expected string
	}{
		{
			name:     "no prefix",
			prefix:   "",
			key:      "stack/component/sha.tfplan",
			expected: "stack/component/sha.tfplan",
		},
		{
			name:     "with prefix",
			prefix:   "planfiles",
			key:      "stack/component/sha.tfplan",
			expected: "planfiles/stack/component/sha.tfplan",
		},
		{
			name:     "nested prefix",
			prefix:   "atmos/ci/plans",
			key:      "dev/vpc/abc.tfplan",
			expected: "atmos/ci/plans/dev/vpc/abc.tfplan",
		},
		{
			name:     "simple key no prefix",
			prefix:   "",
			key:      "test.tfplan",
			expected: "test.tfplan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &Store{prefix: tt.prefix}
			result := store.fullKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewStore_MissingBucket(t *testing.T) {
	tests := []struct {
		name    string
		options map[string]any
	}{
		{
			name:    "nil options",
			options: nil,
		},
		{
			name:    "empty options",
			options: map[string]any{},
		},
		{
			name: "empty bucket",
			options: map[string]any{
				"bucket": "",
			},
		},
		{
			name: "wrong type bucket",
			options: map[string]any{
				"bucket": 123,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewStore(artifact.StoreOptions{
				Options: tt.options,
			})
			assert.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactStoreNotFound)
		})
	}
}

func TestIsNoSuchKeyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "other error",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name:     "NoSuchKey error",
			err:      &types.NoSuchKey{},
			expected: true,
		},
		{
			name:     "wrapped NoSuchKey error",
			err:      &wrappedError{inner: &types.NoSuchKey{}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNoSuchKeyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "other error",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name:     "NotFound error",
			err:      &types.NotFound{},
			expected: true,
		},
		{
			name:     "wrapped NotFound error",
			err:      &wrappedError{inner: &types.NotFound{}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotFoundError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// wrappedError is a helper type for testing error unwrapping.
type wrappedError struct {
	inner error
}

func (e *wrappedError) Error() string {
	return "wrapped: " + e.inner.Error()
}

func (e *wrappedError) Unwrap() error {
	return e.inner
}

func TestStore_MetadataSuffix(t *testing.T) {
	// Verify the metadata suffix constant is correct.
	assert.Equal(t, ".metadata.json", metadataSuffix)
}

func TestStore_StoreName(t *testing.T) {
	// Verify the store name constant is correct.
	assert.Equal(t, "aws/s3", storeName)
}

// fakeAuthContextResolver is a test double for artifact.AuthContextResolver.
type fakeAuthContextResolver struct {
	awsResult       *artifact.AWSAuthConfig
	awsErr          error
	calls           int
	requestedIDName string
}

func (f *fakeAuthContextResolver) ResolveAWSAuthContext(_ context.Context, identityName string) (*store.AWSAuthConfig, error) {
	f.calls++
	f.requestedIDName = identityName
	return f.awsResult, f.awsErr
}

func (f *fakeAuthContextResolver) ResolveAzureAuthContext(_ context.Context, _ string) (*store.AzureAuthConfig, error) {
	return nil, errors.New("not implemented in fake")
}

func (f *fakeAuthContextResolver) ResolveGCPAuthContext(_ context.Context, _ string) (*store.GCPAuthConfig, error) {
	return nil, errors.New("not implemented in fake")
}

// TestNewStore_DeferredInitWhenIdentitySet verifies the client is not
// initialized at construction time when an identity is configured.
func TestNewStore_DeferredInitWhenIdentitySet(t *testing.T) {
	backend, err := NewStore(artifact.StoreOptions{
		Identity: "deploy",
		Options: map[string]any{
			"bucket": "test-bucket",
			"region": "us-east-1",
		},
	})
	require.NoError(t, err)

	s, ok := backend.(*Store)
	require.True(t, ok, "expected *Store concrete type")
	assert.Nil(t, s.client, "client should not be initialized when identity is set")
	assert.Equal(t, "deploy", s.identityName)
	assert.Equal(t, "test-bucket", s.bucket)
	assert.Equal(t, "us-east-1", s.region)
}

// TestNewStore_EagerInitWithoutIdentity verifies the client is initialized
// at construction time when no identity is configured.
func TestNewStore_EagerInitWithoutIdentity(t *testing.T) {
	// LoadDefaultConfig may succeed or fail depending on host env; either
	// outcome proves init was attempted (i.e. not deferred).
	backend, err := NewStore(artifact.StoreOptions{
		Options: map[string]any{
			"bucket": "test-bucket",
			"region": "us-east-1",
		},
	})
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrAWSConfigLoadFailed)
		return
	}
	s, ok := backend.(*Store)
	require.True(t, ok)
	assert.NotNil(t, s.client, "client should be eagerly initialized when no identity is set")
	assert.Empty(t, s.identityName)
}

func TestStore_SetAuthContext(t *testing.T) {
	t.Run("captures resolver, preserves identity when override is empty", func(t *testing.T) {
		s := &Store{identityName: "ci-deployer"}
		resolver := &fakeAuthContextResolver{}

		s.SetAuthContext(resolver, "")

		assert.Same(t, resolver, s.authResolver)
		assert.Equal(t, "ci-deployer", s.identityName, "empty override must not erase existing identity")
	})

	t.Run("non-empty identity overrides existing", func(t *testing.T) {
		s := &Store{identityName: "old-identity"}
		resolver := &fakeAuthContextResolver{}

		s.SetAuthContext(resolver, "new-identity")

		assert.Equal(t, "new-identity", s.identityName)
	})
}

func TestStore_BuildAuthConfigOpts(t *testing.T) {
	tests := []struct {
		name            string
		storeRegion     string
		authContext     *artifact.AWSAuthConfig
		expectedOptsLen int
	}{
		{
			name:            "nil auth context",
			storeRegion:     "",
			authContext:     nil,
			expectedOptsLen: 0,
		},
		{
			name:        "credentials file only",
			storeRegion: "",
			authContext: &artifact.AWSAuthConfig{
				CredentialsFile: "/tmp/creds",
			},
			expectedOptsLen: 1,
		},
		{
			name:        "all fields populated, store region overrides identity region",
			storeRegion: "us-east-1",
			authContext: &artifact.AWSAuthConfig{
				CredentialsFile: "/tmp/creds",
				ConfigFile:      "/tmp/config",
				Profile:         "deploy",
				Region:          "us-west-2", // ignored — store-level region wins
			},
			expectedOptsLen: 4, // creds + config + profile + region
		},
		{
			name:        "identity region used when store region empty",
			storeRegion: "",
			authContext: &artifact.AWSAuthConfig{
				Profile: "deploy",
				Region:  "eu-west-1",
			},
			expectedOptsLen: 2, // profile + region
		},
		{
			name:            "empty auth context produces no opts",
			storeRegion:     "",
			authContext:     &artifact.AWSAuthConfig{},
			expectedOptsLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Store{region: tt.storeRegion}
			got := s.buildAuthConfigOpts(tt.authContext)
			assert.Len(t, got, tt.expectedOptsLen)
		})
	}
}

// TestStore_InitIdentityClient_FallsBackWhenNoResolver verifies that an
// identity-configured backend without an injected resolver falls back to
// the default credential chain rather than failing.
func TestStore_InitIdentityClient_FallsBackWhenNoResolver(t *testing.T) {
	s := &Store{
		bucket:       "test-bucket",
		region:       "us-east-1",
		identityName: "deploy",
	}

	err := s.initIdentityClient(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, s.client)
}

// TestStore_InitIdentityClient_PropagatesResolverError verifies resolver
// errors surface through ensureClient with the expected error sentinel.
func TestStore_InitIdentityClient_PropagatesResolverError(t *testing.T) {
	resolver := &fakeAuthContextResolver{
		awsErr: errors.New("identity chain failed"),
	}
	s := &Store{
		bucket:       "test-bucket",
		region:       "us-east-1",
		identityName: "deploy",
		authResolver: resolver,
	}

	err := s.ensureClient(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAWSConfigLoadFailed)
	assert.Contains(t, err.Error(), "deploy", "error must name the identity for diagnosability")
	assert.Equal(t, 1, resolver.calls)
	assert.Equal(t, "deploy", resolver.requestedIDName)
}

// TestStore_EnsureClient_CallsResolverOnceAcrossOperations verifies the
// sync.Once gate: the resolver is invoked once per backend instance.
func TestStore_EnsureClient_CallsResolverOnceAcrossOperations(t *testing.T) {
	resolver := &fakeAuthContextResolver{
		awsResult: &artifact.AWSAuthConfig{
			Region: "us-east-1",
		},
	}
	s := &Store{
		bucket:       "test-bucket",
		region:       "us-east-1",
		identityName: "deploy",
		authResolver: resolver,
	}

	for range 5 {
		err := s.ensureClient(context.Background())
		require.NoError(t, err)
	}

	assert.Equal(t, 1, resolver.calls, "resolver must only be invoked once")
	assert.NotNil(t, s.client, "client should be populated after first ensureClient call")
}

func TestStore_ImplementsIdentityAwareBackend(t *testing.T) {
	var _ artifact.IdentityAwareBackend = (*Store)(nil) //nolint:gosimple // compile-time assertion
	s := &Store{}
	_, ok := any(s).(artifact.IdentityAwareBackend)
	assert.True(t, ok)
}

func TestStore_QueryToPrefix(t *testing.T) {
	store := &Store{}

	tests := []struct {
		name     string
		query    artifact.Query
		expected string
	}{
		{
			name:     "all query",
			query:    artifact.Query{All: true},
			expected: "",
		},
		{
			name:     "stack only",
			query:    artifact.Query{Stacks: []string{"dev"}},
			expected: "dev",
		},
		{
			name:     "stack and component",
			query:    artifact.Query{Stacks: []string{"dev"}, Components: []string{"vpc"}},
			expected: "dev/vpc",
		},
		{
			name:     "empty query",
			query:    artifact.Query{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.queryToPrefix(tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}
