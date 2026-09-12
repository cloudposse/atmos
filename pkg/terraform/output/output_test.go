package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsComplexValue_DelegatesToSharedOutput verifies the deprecated wrapper
// forwards to pkg/output correctly -- a regression guard against the
// delegation itself, not a re-test of pkg/output's own logic.
func TestIsComplexValue_DelegatesToSharedOutput(t *testing.T) {
	assert.True(t, IsComplexValue(map[string]any{"a": 1}))
	assert.True(t, IsComplexValue([]any{"a"}))
	assert.False(t, IsComplexValue("scalar"))
}

func TestValidateSingleValueFormat_DelegatesToSharedOutput(t *testing.T) {
	err := ValidateSingleValueFormat(map[string]any{"a": 1}, FormatEnv)
	require.Error(t, err)

	err = ValidateSingleValueFormat("scalar", FormatEnv)
	require.NoError(t, err)
}

func TestWriteToFile_DelegatesToSharedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")

	require.NoError(t, WriteToFile(path, "hello"))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}
