package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedoutput "github.com/cloudposse/atmos/pkg/output"
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

// TestSupportedFormats_IsClonedNotAliased guards against SupportedFormats and
// sharedoutput.SupportedFormats sharing a backing array: this package's copy
// must be a clone, so mutating one slice must never corrupt the other's
// format validation and error hints.
func TestSupportedFormats_IsClonedNotAliased(t *testing.T) {
	originalLocal := append([]string(nil), SupportedFormats...)
	originalShared := append([]string(nil), sharedoutput.SupportedFormats...)
	t.Cleanup(func() {
		SupportedFormats = originalLocal
		sharedoutput.SupportedFormats = originalShared
	})

	require.NotEmpty(t, SupportedFormats)
	require.Equal(t, originalShared, SupportedFormats, "must start as an equal-content clone")

	// result -> src isolation: mutating this package's slice must not affect sharedoutput's.
	SupportedFormats[0] = "mutated-local"
	assert.Equal(t, originalShared, sharedoutput.SupportedFormats,
		"mutating this package's SupportedFormats must not corrupt sharedoutput's backing array")

	// src -> result isolation: mutating sharedoutput's slice must not affect the already-cloned copy.
	SupportedFormats = originalLocal
	sharedoutput.SupportedFormats[0] = "mutated-shared"
	assert.Equal(t, originalLocal, SupportedFormats,
		"mutating sharedoutput's SupportedFormats after the clone must not affect this package's copy")
}

// TestScalarOnlyFormats_IsClonedNotAliased is the ScalarOnlyFormats analog of
// TestSupportedFormats_IsClonedNotAliased above.
func TestScalarOnlyFormats_IsClonedNotAliased(t *testing.T) {
	originalLocal := append([]Format(nil), ScalarOnlyFormats...)
	originalShared := append([]Format(nil), sharedoutput.ScalarOnlyFormats...)
	t.Cleanup(func() {
		ScalarOnlyFormats = originalLocal
		sharedoutput.ScalarOnlyFormats = originalShared
	})

	require.NotEmpty(t, ScalarOnlyFormats)
	require.Equal(t, originalShared, ScalarOnlyFormats, "must start as an equal-content clone")

	// result -> src isolation.
	ScalarOnlyFormats[0] = Format("mutated-local")
	assert.Equal(t, originalShared, sharedoutput.ScalarOnlyFormats,
		"mutating this package's ScalarOnlyFormats must not corrupt sharedoutput's backing array")

	// src -> result isolation.
	ScalarOnlyFormats = originalLocal
	sharedoutput.ScalarOnlyFormats[0] = Format("mutated-shared")
	assert.Equal(t, originalLocal, ScalarOnlyFormats,
		"mutating sharedoutput's ScalarOnlyFormats after the clone must not affect this package's copy")
}

func TestWriteToFile_DelegatesToSharedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")

	require.NoError(t, WriteToFile(path, "hello"))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}
