package s3

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
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
