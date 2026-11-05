package batch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhaseSummaries(t *testing.T) {
	output := captureUI(t)
	r := New(0, false, WithPhaseSummaries())
	r.Update(&Event{ID: 1, Phase: "Check", Label: "first.yaml", Diagnostic: true})
	r.Update(&Event{ID: 1, Phase: "Check", Label: "first.yaml", Diagnostic: true, Done: true, Outcome: "checked"})
	r.Update(&Event{ID: 2, Phase: "Check", Label: "second.yaml", Diagnostic: true, Done: true, Outcome: "findings"})
	r.Update(&Event{Reset: true})
	r.Update(&Event{ID: 3, Phase: "Verify", Label: "candidate", Diagnostic: true, Done: true, Outcome: "verification-failed"})
	r.Finish(nil)
	r.Finish(nil)
	assert.Equal(t, "Check · 2 completed · 1 with findings\nVerify · 1 completed · 1 verification failed\n", output.String())
	assert.NotContains(t, output.String(), "yaml")
}

func TestPhaseSummaryFinalizesPartialWork(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"incomplete", nil, "incomplete"}, {"canceled", context.Canceled, "canceled"}, {"failed", errors.New("test error"), "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureUI(t)
			err := Run(0, func(observe Observer) error {
				observe(Event{ID: 1, Phase: "Verify", Label: "candidate", Diagnostic: true})
				return tc.err
			}, WithPhaseSummaries())
			require.ErrorIs(t, err, tc.err)
			assert.Contains(t, output.String(), "Verify · 0 completed · "+tc.want)
			assert.NotContains(t, output.String(), "✓")
			assert.Equal(t, 1, strings.Count(output.String(), "Verify"))
		})
	}
}

func TestPhaseSummaryRetainsLiveJobsAndWarnsOnIncompleteTotals(t *testing.T) {
	output := captureUI(t)
	r := New(2, true, WithPhaseSummaries())
	r.size = func() (int, int, error) { return 80, 24, nil }
	r.Update(&Event{ID: 1, Phase: "Verify", Label: "active candidate", Diagnostic: true, Count: 2})
	assert.Contains(t, output.String(), "active candidate")
	r.Update(&Event{ID: 1, Phase: "Verify", Label: "active candidate", Diagnostic: true, Done: true, Outcome: "passed"})
	r.Finish(nil)
	assert.Contains(t, output.String(), "Verify · 1 completed · incomplete")
	assert.NotContains(t, output.String(), "active candidate (passed)")
}
