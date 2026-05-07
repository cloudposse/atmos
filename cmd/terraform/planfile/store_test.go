package planfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetStoreOptions(t *testing.T) {
	t.Run("explicit S3 store", func(t *testing.T) {
		// Clear environment.
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(&schema.AtmosConfiguration{}, "aws/s3")
		require.NoError(t, err)
		assert.Equal(t, "aws/s3", opts.Type)
	})

	t.Run("explicit local store", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(&schema.AtmosConfiguration{}, "local/dir")
		require.NoError(t, err)
		assert.Equal(t, "local/dir", opts.Type)
	})

	t.Run("S3 from environment", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "my-bucket")
		t.Setenv("ATMOS_PLANFILE_PREFIX", "planfiles")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(nil, "")
		require.NoError(t, err)
		assert.Equal(t, "aws/s3", opts.Type)
		assert.Equal(t, "my-bucket", opts.Options["bucket"])
		assert.Equal(t, "planfiles", opts.Options["prefix"])
		assert.Equal(t, "us-east-1", opts.Options["region"])
	})

	t.Run("GitHub Actions environment", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "true")

		opts, err := getStoreOptions(nil, "")
		require.NoError(t, err)
		assert.Equal(t, "github/artifacts", opts.Type)
	})

	t.Run("default to local", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		opts, err := getStoreOptions(nil, "")
		require.NoError(t, err)
		assert.Equal(t, "local/dir", opts.Type)
		assert.Equal(t, ".atmos/planfiles", opts.Options["path"])
	})

	t.Run("S3 env takes precedence over GitHub Actions", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "my-bucket")
		t.Setenv("GITHUB_ACTIONS", "true")

		opts, err := getStoreOptions(nil, "")
		require.NoError(t, err)
		assert.Equal(t, "aws/s3", opts.Type)
	})

	t.Run("explicit store overrides environment", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "my-bucket")
		t.Setenv("GITHUB_ACTIONS", "true")

		opts, err := getStoreOptions(&schema.AtmosConfiguration{}, "local/dir")
		require.NoError(t, err)
		assert.Equal(t, "local/dir", opts.Type)
	})
}

func TestDefaultKeyPattern(t *testing.T) {
	pattern := planfile.DefaultKeyPattern()
	assert.Contains(t, pattern.Pattern, "{{ .Stack }}")
	assert.Contains(t, pattern.Pattern, "{{ .Component }}")
	assert.Contains(t, pattern.Pattern, "{{ .SHA }}")
}
