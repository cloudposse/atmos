package ci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// summaryMockProvider is a detected provider whose OutputWriter appends to a
// FileOutputWriter pointed at summaryPath, so WriteStepSummary can be exercised
// end to end without depending on a real CI environment.
type summaryMockProvider struct {
	mockProvider
	summaryPath string
}

func (m *summaryMockProvider) OutputWriter() provider.OutputWriter {
	return provider.NewFileOutputWriter("", m.summaryPath)
}

func TestWriteStepSummary_DetectedProviderWritesToSummaryFile(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	summaryPath := filepath.Join(t.TempDir(), "summary.md")
	Register(&summaryMockProvider{
		mockProvider: mockProvider{name: "summary-mock", detected: true},
		summaryPath:  summaryPath,
	})

	const body = "## checkov\n\n✅ no findings\n"
	require.NoError(t, WriteStepSummary(body))

	got, err := os.ReadFile(summaryPath)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

func TestWriteStepSummary_NoProviderIsNoop(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	// No provider registered → no destination, must not error or panic.
	assert.NoError(t, WriteStepSummary("## checkov\n\nfindings\n"))
}

func TestWriteStepSummary_NilOutputWriterIsNoop(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	// mockProvider.OutputWriter() returns nil; WriteStepSummary must guard it.
	Register(&mockProvider{name: "nil-writer", detected: true})
	assert.NoError(t, WriteStepSummary("body"))
}
