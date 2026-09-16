package batch

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestRendererPartialProgressBeforeOrderedCompletion(t *testing.T) {
	output := captureUI(t)
	r := New(3, true)
	r.size = func() (int, int, error) { return 120, 24, nil }
	for id := range 3 {
		r.Update(&Event{ID: id, Label: "same", Phase: "Downloading"})
	}
	cases := []struct {
		name    string
		event   Event
		percent string
		counts  string
	}{
		{"unknown total", Event{ID: 0, Bytes: true, Downloaded: 42}, "0%", "0/3 complete"},
		{"partial download", Event{ID: 0, Bytes: true, Downloaded: 60, Total: 100, Fraction: 0.3}, "10%", "0/3 complete"},
		{"out of order ready", Event{ID: 1, Label: "same", Phase: "Ready", Fraction: 0.5}, "27%", "0/3 complete"},
		{"retry retains progress", Event{ID: 0, Label: "same", Phase: "Retrying", Attempt: 2}, "27%", "0/3 complete"},
		{"retry bytes do not double count", Event{ID: 0, Bytes: true, Fraction: 0.1}, "27%", "0/3 complete"},
		{"late bytes ignored", Event{ID: 99, Bytes: true, Fraction: 0.5}, "27%", "0/3 complete"},
		{"first installed", Event{ID: 0, Label: "same", Done: true, Outcome: "installed"}, "50%", "1/3 complete"},
		{"waiting retains preparation", Event{ID: 1, Label: "same", Phase: "Waiting to install"}, "50%", "1/3 complete"},
		{"installing retains preparation", Event{ID: 1, Label: "same", Phase: "Installing"}, "50%", "1/3 complete"},
		{"failed still counts as processed", Event{ID: 1, Label: "same", Done: true, Outcome: "failed"}, "67%", "2/3 complete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output.Reset()
			r.Update(&tc.event)
			r.Tick()
			assert.Contains(t, ansi.Strip(output.String()), tc.percent+" "+tc.counts)
		})
	}
	r.Update(&Event{Reset: true, Count: 1})
	output.Reset()
	r.Update(&Event{ID: 0, Label: "next phase", Phase: "Checking"})
	assert.Contains(t, ansi.Strip(output.String()), "0% 0/1 complete")
}

func TestRendererClampsPartialProgress(t *testing.T) {
	output := captureUI(t)
	r := New(2, true)
	r.size = func() (int, int, error) { return 120, 24, nil }
	r.Update(&Event{ID: 0, Label: "negative", Fraction: -1})
	assert.Contains(t, ansi.Strip(output.String()), "0% 0/2 complete")
	output.Reset()
	r.Update(&Event{ID: 0, Bytes: true, Fraction: 2})
	r.Tick()
	assert.Contains(t, ansi.Strip(output.String()), "50% 0/2 complete")
	output.Reset()
	r.Update(&Event{ID: 1, Label: "excess", Fraction: 2})
	assert.Contains(t, ansi.Strip(output.String()), "100% 0/2 complete")
}

func TestToolchainRetainsCompletionBasedBar(t *testing.T) {
	output := captureUI(t)
	r := New(2, true, WithToolchainStyle())
	r.size = func() (int, int, error) { return 120, 24, nil }
	r.Update(&Event{ID: 0, Label: "terraform", Phase: "Downloading", Fraction: 0.5})
	assert.Contains(t, ansi.Strip(output.String()), "0% 0/2 complete, 1 running")
}
