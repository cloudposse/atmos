package batch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func captureUI(t *testing.T) *bytes.Buffer {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	ctx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ctx)
	t.Cleanup(ui.Reset)
	var output bytes.Buffer
	t.Cleanup(iolib.PushUIWriter(&output))
	return &output
}

func TestRendererPlainResults(t *testing.T) {
	output := captureUI(t)
	r := New(6, false)
	cases := []struct {
		label, outcome string
		err            error
	}{
		{"vpc", "installed", nil},
		{"eks", "unchanged", nil},
		{"providers", "skipped", nil},
		{"rds", "canceled", nil},
		{"bad-source", "", errors.New("download failed")},
		{"cluster", "updated", nil},
	}
	for i, tc := range cases {
		r.Update(&Event{ID: i, Label: tc.label, Phase: "Downloading"})
		r.Tick()
		r.Update(&Event{ID: i, Label: tc.label, Outcome: tc.outcome, Done: true, Err: tc.err})
	}
	text := ansi.Strip(output.String())
	for _, tc := range cases {
		assert.Contains(t, text, tc.label)
	}
	assert.Contains(t, text, "eks (unchanged)")
	assert.Contains(t, text, "providers (skipped)")
	assert.Contains(t, text, "rds (canceled)")
	assert.Contains(t, text, "bad-source: download failed")
	assert.Contains(t, text, "cluster (updated)")
	assert.NotContains(t, output.String(), "\x1b[")
	assert.NotContains(t, text, "Downloading")
	assert.Equal(t, 6, strings.Count(strings.TrimSpace(text), "\n")+1)
}

func TestRendererDuplicateLabelsAndByteUpdates(t *testing.T) {
	output := captureUI(t)
	r := New(4, true)
	r.size = func() (int, int, error) { return 100, 24, nil }
	r.Update(&Event{ID: 10, Label: "same", Phase: "Downloading"})
	r.Update(&Event{ID: 11, Label: "same", Phase: "Preparing"})
	output.Reset()
	// Byte-only events must preserve the phase and label, and ignore unknown IDs.
	r.Update(&Event{ID: 10, Bytes: true, Downloaded: 1024, Total: 2048})
	r.Update(&Event{ID: 99, Bytes: true, Downloaded: 4096})
	r.Tick()
	text := ansi.Strip(output.String())
	assert.Contains(t, text, "Downloading same")
	assert.Contains(t, text, "Preparing same")
	assert.Contains(t, text, "1.0 KB/2.0 KB")
	assert.Contains(t, text, "0/4 complete, 2 active, 2 queued")
	output.Reset()
	r.Update(&Event{ID: 10, Done: true, Label: "same", Outcome: "installed"})
	text = ansi.Strip(output.String())
	assert.NotContains(t, text, "Downloading same")
	assert.Contains(t, text, "Preparing same")
	assert.Contains(t, text, "1/4 complete, 1 active, 2 queued")
	output.Reset()
	r.Update(&Event{ID: 11, Label: "same", Phase: "Retrying", Attempt: 2, Count: 3})
	text = ansi.Strip(output.String())
	assert.Contains(t, text, "Retrying same (attempt 2)")
	assert.Contains(t, text, "1/3 complete, 1 active, 1 queued")
	r.Clear()
	output.Reset()
	r.Clear()
	assert.Empty(t, output.String(), "repeated cleanup must not erase permanent results")
}

func TestRendererResizesAndLimitsActiveRows(t *testing.T) {
	output := captureUI(t)
	r := New(4, true)
	width, height := 80, 24
	r.size = func() (int, int, error) { return width, height, nil }
	for i := range 4 {
		r.Update(&Event{ID: i, Label: fmt.Sprintf("component-%d-with-a-very-long-label", i), Phase: "Downloading"})
	}
	output.Reset()
	width, height = 24, 5
	r.Tick()
	text := ansi.Strip(output.String())
	assert.Contains(t, text, "… 2 more active")
	assert.NotContains(t, text, "component-2")
	assert.NotContains(t, text, "component-3")
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), width-1)
	}
	output.Reset()
	width, height = 100, 24
	r.Tick()
	text = ansi.Strip(output.String())
	assert.Contains(t, text, "component-3-with-a-very-long-label")
	assert.NotContains(t, text, "more active")
	assert.Contains(t, text, "0/4 complete, 4 active, 0 queued")
}

func TestRendererSizeFallbackAndReset(t *testing.T) {
	output := captureUI(t)
	r := New(3, true)
	r.size = func() (int, int, error) { return 0, 0, errors.New("not a terminal") }
	r.Update(&Event{ID: 1, Label: "old", Phase: "Checking"})
	assert.Contains(t, ansi.Strip(output.String()), "Checking old")
	r.Update(&Event{ID: 0, Label: "finished", Done: true})
	output.Reset()
	r.Update(&Event{Reset: true, Count: 2})
	assert.Contains(t, output.String(), "\x1b[J")
	output.Reset()
	r.Update(&Event{ID: 0, Label: "new", Phase: "Downloading"})
	text := ansi.Strip(output.String())
	assert.Contains(t, text, "0/2 complete, 1 active, 1 queued")
	assert.NotContains(t, text, "old")
	assert.Contains(t, text, "Downloading new")
}

func TestRunDrainsConcurrentProducersAndReturnsFailure(t *testing.T) {
	output := captureUI(t)
	t.Setenv("TERM", "dumb")
	const count = 20
	expected := errors.New("batch failed")
	err := Run(count, func(observe Observer) error {
		var workers sync.WaitGroup
		for id := range count {
			workers.Add(1)
			go func() {
				defer workers.Done()
				label := fmt.Sprintf("package-%02d", id)
				observe(Event{ID: id, Label: label, Phase: "Downloading"})
				for n := range 100 {
					observe(Event{ID: id, Bytes: true, Downloaded: int64(n), Total: 100})
				}
				observe(Event{ID: id, Label: label, Done: true, Outcome: "installed"})
			}()
		}
		workers.Wait()
		return expected
	})
	require.ErrorIs(t, err, expected)
	text := ansi.Strip(output.String())
	for id := range count {
		assert.Equal(t, 1, strings.Count(text, fmt.Sprintf("package-%02d", id)), "every terminal event must be rendered once")
	}
	assert.NotContains(t, output.String(), "\x1b[")
}

func TestRunCancellationRetainsTerminalResult(t *testing.T) {
	output := captureUI(t)
	t.Setenv("TERM", "dumb")
	err := Run(1, func(observe Observer) error {
		observe(Event{ID: 1, Label: "canceled-job", Phase: "Downloading"})
		observe(Event{ID: 1, Label: "canceled-job", Done: true, Outcome: "canceled"})
		return context.Canceled
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, ansi.Strip(output.String()), "canceled-job (canceled)")
}

func TestAlignProgress(t *testing.T) {
	cases := []struct {
		name, left        string
		downloaded, total int64
		width             int
		right             string
	}{
		{"unknown size", "Downloading", 0, 0, 50, ""},
		{"bytes only", "Downloading", 42, 0, 50, "42 B"},
		{"known total", "Downloading", 1024, 2048, 50, "1.0 KB/2.0 KB"},
		{"zero downloaded", "Downloading", 0, 2048, 50, "0 B/2.0 KB"},
		{"too narrow", "Downloading", 1024, 2048, 12, ""},
		{"unicode", "⠋ 準備中", 1024 * 1024, 2 * 1024 * 1024, 50, "1.0 MB/2.0 MB"},
		{"gigabytes", "Downloading", 1 << 30, 0, 50, "1.0 GB"},
		{"terabytes", "Downloading", 1 << 40, 0, 50, "1.0 TB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AlignProgress(tc.left, tc.downloaded, tc.total, tc.width)
			if tc.right == "" {
				assert.Equal(t, tc.left, got)
			} else {
				assert.True(t, strings.HasSuffix(got, tc.right))
				assert.Equal(t, tc.width, lipgloss.Width(got))
				assert.True(t, strings.HasPrefix(got, tc.left+" "))
			}
		})
	}
}

func TestToolchainStylePreservesCustomResults(t *testing.T) {
	output := captureUI(t)
	r := New(3, true, WithToolchainStyle())
	r.size = func() (int, int, error) { return 100, 24, nil }
	r.Update(&Event{ID: 0, Label: "terraform@1.0"})
	r.Update(&Event{ID: 1, Label: "tofu@2.0"})
	output.Reset()
	r.Complete(0, func() { ui.Writef("custom terraform installed at /tools/terraform\n") })
	text := ansi.Strip(output.String())
	assert.Contains(t, text, "custom terraform installed at /tools/terraform")
	assert.Contains(t, text, "tofu@2.0")
	assert.Contains(t, text, "1/3 complete, 1 running")
	assert.NotContains(t, text, "queued")
	assert.Less(t, strings.Index(text, "custom terraform"), strings.Index(text, "tofu@2.0"))
}

func TestRendererVeryNarrowTerminalClipsOverflowSummary(t *testing.T) {
	output := captureUI(t)
	r := New(3, true)
	r.size = func() (int, int, error) { return 6, 4, nil }
	for id := range 3 {
		r.Update(&Event{ID: id, Label: "component", Phase: "Downloading"})
	}
	output.Reset()
	r.Tick()
	for _, line := range strings.Split(strings.TrimSpace(ansi.Strip(output.String())), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 5)
	}
}

func TestRunHonorsForcedTTYAndUnsupportedTerminals(t *testing.T) {
	previous := viper.Get("force-tty")
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", previous) })
	for _, terminalName := range []string{"xterm-256color", "dumb", "unknown"} {
		t.Run(terminalName, func(t *testing.T) {
			output := captureUI(t)
			t.Setenv("TERM", terminalName)
			t.Setenv("ATMOS_CAST_RECORDING_WIDTH", "50")
			require.NoError(t, Run(1, func(observe Observer) error {
				observe(Event{ID: 1, Label: "recorded-job", Phase: "Downloading"})
				observe(Event{ID: 1, Label: "recorded-job", Done: true})
				return nil
			}))
			text := ansi.Strip(output.String())
			assert.Contains(t, text, "recorded-job")
			if terminalName == "xterm-256color" {
				assert.Contains(t, text, "Downloading recorded-job")
				assert.Contains(t, output.String(), "\x1b[J")
				for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
					assert.LessOrEqual(t, lipgloss.Width(line), 49)
				}
			} else {
				assert.NotContains(t, text, "Downloading")
				assert.NotContains(t, output.String(), "\x1b[")
			}
		})
	}
}

func TestRendererSnapshotsCallerEvent(t *testing.T) {
	output := captureUI(t)
	renderer := New(1, true)
	renderer.size = func() (int, int, error) { return 100, 24, nil }
	event := Event{ID: 1, Label: "original", Phase: "Downloading", Downloaded: 12, Total: 24}
	renderer.Update(&event)
	event.Label = "caller changed label"
	event.Downloaded = 999
	output.Reset()
	renderer.Tick()
	text := ansi.Strip(output.String())
	assert.Contains(t, text, "Downloading original")
	assert.Contains(t, text, "12 B/24 B")
	assert.NotContains(t, text, "caller changed label")
	assert.NotContains(t, text, "999 B")
	renderer.Update(&Event{ID: 1, Bytes: true, Downloaded: 18, Total: 24})
	assert.Equal(t, "caller changed label", event.Label)
	assert.Equal(t, int64(999), event.Downloaded, "renderer progress updates must not mutate the caller's event")
}

func TestRendererWarningPreservesActiveJobAndCounts(t *testing.T) {
	output := captureUI(t)
	renderer := New(2, true)
	renderer.size = func() (int, int, error) { return 100, 24, nil }
	renderer.Update(&Event{ID: 0, Label: "vpc", Phase: "Checking"})
	output.Reset()
	renderer.Update(&Event{Warning: "Vendor lock drift detected for vpc"})
	text := ansi.Strip(output.String())
	assert.Contains(t, text, "Vendor lock drift detected for vpc")
	assert.Contains(t, text, "Checking vpc")
	assert.Contains(t, text, "0/2 complete, 1 active, 1 queued")
	assert.Zero(t, renderer.completed)
	require.Len(t, renderer.active, 1)
}

func TestRendererVeryShortTerminalReservesFooter(t *testing.T) {
	output := captureUI(t)
	renderer := New(3, true)
	height := 1
	renderer.size = func() (int, int, error) { return 100, height, nil }
	for id := range 3 {
		renderer.Update(&Event{ID: id, Label: "component", Phase: "Downloading"})
	}
	for height = 1; height <= 4; height++ {
		output.Reset()
		renderer.Tick()
		assert.LessOrEqual(t, renderer.lines, max(1, height-1))
		assert.Contains(t, ansi.Strip(output.String()), "0/3 complete, 3 active, 0 queued")
	}
}
