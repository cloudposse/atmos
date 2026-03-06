package planfile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBuildUploadMetadata(t *testing.T) {
	t.Run("with all fields set", func(t *testing.T) {
		opts := &UploadOptions{
			Component: "test-component",
		}
		opts.Stack = "test-stack"

		metadata := buildUploadMetadata(opts, "abc123")
		require.NotNil(t, metadata)
		assert.Equal(t, "test-stack", metadata.Stack)
		assert.Equal(t, "test-component", metadata.Component)
		// Explicit SHA takes precedence over auto-detected SHA.
		assert.Equal(t, "abc123", metadata.SHA)
		assert.False(t, metadata.CreatedAt.IsZero())
		// CreatedAt should be recent.
		assert.WithinDuration(t, time.Now(), metadata.CreatedAt, 5*time.Second)
		// AtmosVersion is always populated.
		assert.NotEmpty(t, metadata.AtmosVersion)
	})

	t.Run("with empty fields auto-detects CI context", func(t *testing.T) {
		opts := &UploadOptions{
			Component: "",
		}
		opts.Stack = ""

		metadata := buildUploadMetadata(opts, "")
		require.NotNil(t, metadata)
		assert.Empty(t, metadata.Stack)
		assert.Empty(t, metadata.Component)
		// SHA is auto-detected from git when not provided via flag.
		// Branch and AtmosVersion are also populated from CI context.
		assert.NotEmpty(t, metadata.AtmosVersion)
	})
}

func TestResolveUploadPlanfilePath(t *testing.T) {
	t.Run("explicit planfile path", func(t *testing.T) {
		opts := &UploadOptions{
			PlanfilePath: "/tmp/my-plan.tfplan",
			Component:    "vpc",
		}
		opts.Stack = "dev"

		path, err := resolveUploadPlanfilePath(opts, nil)
		require.NoError(t, err)
		assert.Equal(t, "/tmp/my-plan.tfplan", path)
	})

	t.Run("missing planfile and component", func(t *testing.T) {
		opts := &UploadOptions{
			PlanfilePath: "",
			Component:    "",
		}
		opts.Stack = "dev"

		_, err := resolveUploadPlanfilePath(opts, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--planfile is required")
	})

	t.Run("missing planfile and stack", func(t *testing.T) {
		opts := &UploadOptions{
			PlanfilePath: "",
			Component:    "vpc",
		}
		opts.Stack = ""

		_, err := resolveUploadPlanfilePath(opts, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--planfile is required")
	})

	t.Run("missing planfile stack and component", func(t *testing.T) {
		opts := &UploadOptions{
			PlanfilePath: "",
			Component:    "",
		}
		opts.Stack = ""

		_, err := resolveUploadPlanfilePath(opts, nil)
		require.Error(t, err)
	})
}

func TestGetStoreOptions(t *testing.T) {
	t.Run("explicit S3 store", func(t *testing.T) {
		// Clear environment.
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(&schema.AtmosConfiguration{}, "s3")
		require.NoError(t, err)
		assert.Equal(t, "s3", opts.Type)
	})

	t.Run("explicit local store", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(&schema.AtmosConfiguration{}, "local")
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

		opts, err := getStoreOptions(&schema.AtmosConfiguration{}, "local")
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
