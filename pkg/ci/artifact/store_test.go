package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Compile-time interface checks.
var _ Backend = (*testBackend)(nil)

func TestStoreOptionsDefaults(t *testing.T) {
	opts := StoreOptions{
		Type: "s3",
		Options: map[string]any{
			"bucket": "my-bucket",
			"region": "us-east-1",
		},
	}

	assert.Equal(t, "s3", opts.Type)
	assert.Equal(t, "my-bucket", opts.Options["bucket"])
	assert.Nil(t, opts.AtmosConfig)
}

func TestFileEntryFields(t *testing.T) {
	entry := FileEntry{
		Name: "plan.tfplan",
		Data: nil,
		Size: 1024,
	}

	assert.Equal(t, "plan.tfplan", entry.Name)
	assert.Equal(t, int64(1024), entry.Size)
}

func TestFileResultFields(t *testing.T) {
	result := FileResult{
		Name: "plan.tfplan",
		Data: nil,
		Size: 2048,
	}

	assert.Equal(t, "plan.tfplan", result.Name)
	assert.Equal(t, int64(2048), result.Size)
}
