package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTerraformBackendConfigToFileAsHclTruncatesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backend.tf")

	require.NoError(t, WriteTerraformBackendConfigToFileAsHcl(path, "s3", map[string]any{
		"bucket":         "a-very-long-bucket-name-used-by-the-first-generation",
		"dynamodb_table": "terraform-state-lock",
		"key":            "terraform.tfstate",
		"region":         "us-east-2",
	}))

	require.NoError(t, WriteTerraformBackendConfigToFileAsHcl(path, "s3", map[string]any{
		"bucket": "short",
	}))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "terraform {\n  backend \"s3\" {\n    bucket = \"short\"\n  }\n}\n", string(got))
}
