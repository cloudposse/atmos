package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

func TestSelectedComponentsUseOneBatch(t *testing.T) {
	file := selectionBatchFixture(t)
	var events []batch.Event
	var enumerated []string
	calls := 0
	report, err := UpdateSelectedComponents(&SelectionParams{
		VendorFile: file, MaxConcurrency: 2, Check: true, Context: context.Background(),
		OnEvent: func(e batch.Event) { events = append(events, e) },
		RunWithProgress: func(work func(func(string, int, int)) (*vendoring.UpdateReport, error)) (*vendoring.UpdateReport, error) {
			calls++
			return work(func(component string, index, total int) {
				enumerated = append(enumerated, component)
				assert.Equal(t, len(enumerated), index)
				assert.Equal(t, 2, total)
			})
		},
	}, []string{"second", "first"})
	require.NoError(t, err)
	assert.Equal(t, 1, calls, "progress wrapper owns the whole selection")
	assert.Equal(t, []string{"second", "first"}, enumerated)
	require.Len(t, report.Results, 2)
	assert.Equal(t, "second", report.Results[0].Component)
	assert.Equal(t, "first", report.Results[1].Component)
	for _, result := range report.Results {
		assert.Equal(t, vendoring.StatusSkipped, result.Status)
	}
	resets := 0
	for _, e := range events {
		if e.Reset {
			resets++
			assert.Equal(t, 2, e.Count)
		}
	}
	assert.Equal(t, 1, resets, "one reset covers all components")
}

func TestSelectedComponentsContextAndLimit(t *testing.T) {
	for _, test := range []struct {
		name   string
		cancel bool
		limit  int
		want   error
	}{
		{name: "caller cancellation", cancel: true, limit: 2, want: context.Canceled},
		{name: "invalid concurrency", limit: -1, want: errUtils.ErrInvalidFlagValue},
		{name: "without progress wrapper", limit: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := selectionBatchFixture(t)
			before, err := os.ReadFile(file)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancel {
				cancel()
			}
			report, err := UpdateSelectedComponents(&SelectionParams{VendorFile: file, Context: ctx, MaxConcurrency: test.limit, Check: true}, []string{"first", "second"})
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
			} else {
				require.NoError(t, err)
				require.Len(t, report.Results, 2)
				assert.Equal(t, vendoring.StatusSkipped, report.Results[0].Status)
				assert.Equal(t, "second", report.Results[1].Component)
			}
			if test.cancel {
				require.Len(t, report.Results, 2)
				assert.Equal(t, vendoring.StatusFailed, report.Results[0].Status)
				assert.Equal(t, vendoring.StatusFailed, report.Results[1].Status)
			}
			after, err := os.ReadFile(file)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func selectionBatchFixture(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(file, []byte("spec:\n  sources:\n    - component: first\n      source: oci://example.com/first\n      version: '{{.Version}}'\n    - component: second\n      source: oci://example.com/second\n      version: '{{.Version}}'\n"), 0o644))
	return file
}
