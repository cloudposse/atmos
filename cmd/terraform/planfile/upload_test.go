package planfile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
