package planfile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/planfile"
)

func TestBuildUploadMetadata(t *testing.T) {
	// Save original values.
	originalStack := uploadStack
	originalComponent := uploadComponent
	originalSHA := uploadSHA

	// Restore after test.
	defer func() {
		uploadStack = originalStack
		uploadComponent = originalComponent
		uploadSHA = originalSHA
	}()

	t.Run("with all fields set", func(t *testing.T) {
		uploadStack = "test-stack"
		uploadComponent = "test-component"
		uploadSHA = "abc123"

		metadata := buildUploadMetadata()
		require.NotNil(t, metadata)
		assert.Equal(t, "test-stack", metadata.Stack)
		assert.Equal(t, "test-component", metadata.Component)
		assert.Equal(t, "abc123", metadata.SHA)
		assert.False(t, metadata.CreatedAt.IsZero())
		// CreatedAt should be recent.
		assert.WithinDuration(t, time.Now(), metadata.CreatedAt, 5*time.Second)
	})

	t.Run("with empty fields", func(t *testing.T) {
		uploadStack = ""
		uploadComponent = ""
		uploadSHA = ""

		metadata := buildUploadMetadata()
		require.NotNil(t, metadata)
		assert.Empty(t, metadata.Stack)
		assert.Empty(t, metadata.Component)
		assert.Empty(t, metadata.SHA)
	})
}

func TestResolveUploadKey(t *testing.T) {
	// Save original values.
	originalKey := uploadKey
	originalStack := uploadStack
	originalComponent := uploadComponent
	originalSHA := uploadSHA

	// Restore after test.
	defer func() {
		uploadKey = originalKey
		uploadStack = originalStack
		uploadComponent = originalComponent
		uploadSHA = originalSHA
	}()

	t.Run("explicit key provided", func(t *testing.T) {
		uploadKey = "custom/key.tfplan"
		uploadStack = "ignored"
		uploadComponent = "ignored"
		uploadSHA = "ignored"

		key, err := resolveUploadKey()
		require.NoError(t, err)
		assert.Equal(t, "custom/key.tfplan", key)
	})

	t.Run("generated key from metadata", func(t *testing.T) {
		uploadKey = ""
		uploadStack = "my-stack"
		uploadComponent = "my-component"
		uploadSHA = "def456"

		key, err := resolveUploadKey()
		require.NoError(t, err)
		assert.Equal(t, "my-stack/my-component/def456.tfplan", key)
	})

	t.Run("missing required fields for generated key", func(t *testing.T) {
		uploadKey = ""
		uploadStack = ""
		uploadComponent = "component"
		uploadSHA = "sha123"

		_, err := resolveUploadKey()
		assert.Error(t, err)
	})
}

func TestGetStoreOptions(t *testing.T) {
	t.Run("explicit S3 store", func(t *testing.T) {
		// Clear environment.
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(nil, "s3")
		require.NoError(t, err)
		assert.Equal(t, "s3", opts.Type)
	})

	t.Run("explicit local store", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(nil, "local")
		require.NoError(t, err)
		assert.Equal(t, "local", opts.Type)
	})

	t.Run("S3 from environment", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "my-bucket")
		t.Setenv("ATMOS_PLANFILE_PREFIX", "planfiles")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(nil, "")
		require.NoError(t, err)
		assert.Equal(t, "s3", opts.Type)
		assert.Equal(t, "my-bucket", opts.Options["bucket"])
		assert.Equal(t, "planfiles", opts.Options["prefix"])
		assert.Equal(t, "us-east-1", opts.Options["region"])
	})

	t.Run("GitHub Actions environment", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "true")

		opts, err := getStoreOptions(nil, "")
		require.NoError(t, err)
		assert.Equal(t, "github-artifacts", opts.Type)
	})

	t.Run("default to local", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(nil, "")
		require.NoError(t, err)
		assert.Equal(t, "local", opts.Type)
		assert.Equal(t, ".atmos/planfiles", opts.Options["path"])
	})

	t.Run("S3 env takes precedence over GitHub Actions", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "my-bucket")
		t.Setenv("GITHUB_ACTIONS", "true")

		opts, err := getStoreOptions(nil, "")
		require.NoError(t, err)
		assert.Equal(t, "s3", opts.Type)
	})

	t.Run("explicit store overrides environment", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "my-bucket")
		t.Setenv("GITHUB_ACTIONS", "true")

		opts, err := getStoreOptions(nil, "local")
		require.NoError(t, err)
		assert.Equal(t, "local", opts.Type)
	})
}

func TestDefaultKeyPattern(t *testing.T) {
	pattern := planfile.DefaultKeyPattern()
	assert.Contains(t, pattern.Pattern, "{{ .Stack }}")
	assert.Contains(t, pattern.Pattern, "{{ .Component }}")
	assert.Contains(t, pattern.Pattern, "{{ .SHA }}")
}
