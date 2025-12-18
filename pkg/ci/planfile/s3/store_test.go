package s3

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
)

func TestStore_Name(t *testing.T) {
	store := &Store{
		bucket: "test-bucket",
	}
	assert.Equal(t, "s3", store.Name())
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
			_, err := NewStore(planfile.StoreOptions{
				Options: tt.options,
			})
			assert.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrPlanfileStoreNotFound)
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

func TestErrorAs(t *testing.T) {
	t.Run("finds NoSuchKey directly", func(t *testing.T) {
		err := &types.NoSuchKey{}
		var target *types.NoSuchKey
		assert.True(t, errorAs(err, &target))
		assert.NotNil(t, target)
	})

	t.Run("finds NotFound directly", func(t *testing.T) {
		err := &types.NotFound{}
		var target *types.NotFound
		assert.True(t, errorAs(err, &target))
		assert.NotNil(t, target)
	})

	t.Run("finds wrapped NoSuchKey", func(t *testing.T) {
		err := &wrappedError{inner: &types.NoSuchKey{}}
		var target *types.NoSuchKey
		assert.True(t, errorAs(err, &target))
		assert.NotNil(t, target)
	})

	t.Run("not found in different error", func(t *testing.T) {
		err := errors.New("different error")
		var target *types.NoSuchKey
		assert.False(t, errorAs(err, &target))
	})

	t.Run("nil error returns false", func(t *testing.T) {
		var target *types.NoSuchKey
		assert.False(t, errorAs(nil, &target))
	})

	t.Run("deeply wrapped error", func(t *testing.T) {
		err := &wrappedError{
			inner: &wrappedError{
				inner: &types.NoSuchKey{},
			},
		}
		var target *types.NoSuchKey
		assert.True(t, errorAs(err, &target))
	})
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

// Note: Testing actual S3 operations (Upload, Download, Delete, List, Exists, GetMetadata)
// requires either:
// 1. An interface abstraction for the S3 client (recommended for unit tests)
// 2. Integration tests with LocalStack or a real S3 bucket
//
// The following tests document the expected behavior and can be enabled
// when running against a real S3 backend or LocalStack.

func TestStore_MetadataSuffix(t *testing.T) {
	// Verify the metadata suffix constant is correct.
	assert.Equal(t, ".metadata.json", metadataSuffix)
}

func TestStore_StoreName(t *testing.T) {
	// Verify the store name constant is correct.
	assert.Equal(t, "s3", storeName)
}

// Integration tests would go here with build tag: //go:build integration

/*
Example integration test structure (requires LocalStack or real S3):

func TestIntegration_FullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup: Create store with test bucket
	store, err := NewStore(planfile.StoreOptions{
		Options: map[string]any{
			"bucket": os.Getenv("TEST_S3_BUCKET"),
			"region": os.Getenv("AWS_REGION"),
			"prefix": "test-planfiles",
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	key := fmt.Sprintf("test/%s/lifecycle.tfplan", uuid.New().String())

	// Test Upload
	metadata := &planfile.Metadata{
		Stack:      "test-stack",
		Component:  "test-component",
		SHA:        "abc123",
		HasChanges: true,
	}
	err = store.Upload(ctx, key, strings.NewReader("plan data"), metadata)
	assert.NoError(t, err)

	// Test Exists
	exists, err := store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test GetMetadata
	meta, err := store.GetMetadata(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, "test-stack", meta.Stack)

	// Test Download
	reader, downloadMeta, err := store.Download(ctx, key)
	assert.NoError(t, err)
	defer reader.Close()
	content, _ := io.ReadAll(reader)
	assert.Equal(t, "plan data", string(content))
	assert.Equal(t, "test-stack", downloadMeta.Stack)

	// Test List
	files, err := store.List(ctx, "test/")
	assert.NoError(t, err)
	assert.Greater(t, len(files), 0)

	// Test Delete
	err = store.Delete(ctx, key)
	assert.NoError(t, err)

	// Verify deleted
	exists, err = store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.False(t, exists)
}
*/
