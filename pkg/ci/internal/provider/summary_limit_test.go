package provider

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
)

func TestFileOutputWriterSummarySizeLimit(t *testing.T) {
	const limit = 1024 * 1024
	for _, tc := range []struct {
		name     string
		existing string
		incoming string
		wantErr  bool
	}{
		{name: "exact content budget", existing: strings.Repeat("x", limit-len(SummarySizeWarning)-3), incoming: "end"},
		{name: "cumulative overflow", existing: strings.Repeat("x", limit-2), incoming: "end", wantErr: true},
		{name: "oversized first summary", incoming: strings.Repeat("x", limit+1), wantErr: true},
		{name: "UTF-8 bytes", existing: strings.Repeat("x", limit-1), incoming: "é", wantErr: true},
		{name: "already oversized", existing: strings.Repeat("x", limit+1), incoming: "more", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "summary.md")
			if tc.existing != "" {
				require.NoError(t, os.WriteFile(path, []byte(tc.existing), 0o600))
			}
			err := (&FileOutputWriter{SummaryPath: path}).WriteSummary(tc.incoming)
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
				assert.Contains(t, err.Error(), "1 MiB")
			} else {
				require.NoError(t, err)
			}
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			want := tc.existing
			if !tc.wantErr {
				want += tc.incoming
			} else if len(tc.existing)+len(SummarySizeWarning) <= limit {
				want += SummarySizeWarning
			}
			assert.Equal(t, want, string(content))
		})
	}
}

func TestFileOutputWriterSummaryLimitUsesMaskedBytes(t *testing.T) {
	atmosio.Reset()
	t.Cleanup(atmosio.Reset)
	const secret = "X!z"
	atmosio.RegisterSecret(secret)
	masked := MaskPublishedContent(secret)
	require.Greater(t, len(masked), len(secret))
	path := filepath.Join(t.TempDir(), "summary.md")
	existing := strings.Repeat("x", 1024*1024-len(SummarySizeWarning)-len(secret))
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o600))
	err := (&FileOutputWriter{SummaryPath: path}).WriteSummary(secret)
	require.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, existing+SummarySizeWarning, string(content))
}

func TestFileOutputWriterConcurrentSummariesRespectLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.md")
	const chunkSize = 768 * 1024
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			results <- (&FileOutputWriter{SummaryPath: path}).WriteSummary(strings.Repeat("x", chunkSize))
		})
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			require.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
		}
	}
	assert.Equal(t, 1, succeeded)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.EqualValues(t, chunkSize+len(SummarySizeWarning), info.Size())
}

func TestFileOutputWriterSummaryIOErrors(t *testing.T) {
	t.Run("lock cannot be created", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "summary.md")
		err := (&FileOutputWriter{SummaryPath: path}).WriteSummary("summary")
		require.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
	})
	t.Run("summary is a directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "summary.md")
		require.NoError(t, os.Mkdir(path, 0o755))
		err := (&FileOutputWriter{SummaryPath: path}).WriteSummary("summary")
		require.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
		err = (&FileOutputWriter{SummaryPath: path}).WriteSummary(strings.Repeat("x", 1024*1024))
		require.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestFileOutputWriterSummaryOverflowAdmonition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.md")
	writer := &FileOutputWriter{SummaryPath: path}
	const earlier = "## Earlier summary\n\nCompleted successfully.\n"
	require.NoError(t, writer.WriteSummary(earlier))
	for range 2 {
		err := writer.WriteSummary(strings.Repeat("x", 1024*1024))
		require.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
	}
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(content), earlier))
	assert.Equal(t, 1, strings.Count(string(content), "> [!WARNING]"))
	assert.Contains(t, string(content), "Output exceeds the maximum summary size.")
	assert.Contains(t, string(content), "Consider reducing scope using `--tags` or `--labels`.")
	assert.LessOrEqual(t, len(content), 1024*1024)
}

func TestFileOutputWriterReservesSpaceForSummaryWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.md")
	writer := &FileOutputWriter{SummaryPath: path}
	payload := strings.Repeat("x", 1024*1024-len(SummarySizeWarning))
	require.NoError(t, writer.WriteSummary(payload))
	require.ErrorIs(t, writer.WriteSummary("overflow"), errUtils.ErrCISummaryWriteFailed)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, payload+SummarySizeWarning, string(content))
	assert.Equal(t, 1024*1024, len(content))
}

func TestFileOutputWriterUsesFullBudgetAfterSummaryWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.md")
	writer := &FileOutputWriter{SummaryPath: path}
	const earlier = "## Earlier summary\n"
	require.NoError(t, writer.WriteSummary(earlier))
	require.ErrorIs(t, writer.WriteSummary(strings.Repeat("x", 1024*1024)), errUtils.ErrCISummaryWriteFailed)

	remaining := strings.Repeat("y", 1024*1024-len(earlier)-len(SummarySizeWarning))
	require.NoError(t, writer.WriteSummary(remaining))
	require.ErrorIs(t, writer.WriteSummary("overflow"), errUtils.ErrCISummaryWriteFailed)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, earlier+SummarySizeWarning+remaining, string(content))
	assert.Equal(t, 1024*1024, len(content))
}
