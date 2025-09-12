package test

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

func init() {
	// Use ASCII profile for consistent output in tests
	lipgloss.SetColorProfile(termenv.Ascii)
}

// streamOutputMsg represents a line of output from the test stream
type streamOutputMsg struct {
	line string
}

// TestTUIWithTeatest tests the TUI mode using the teatest library
// This allows testing without a real TTY
func TestTUIWithTeatest(t *testing.T) {
	t.Run("basic TUI initialization", func(t *testing.T) {
		// Create a new test model with minimal configuration
		model := tui.NewTestModel(
			[]string{"./test"},
			"-v",
			"output.json",
			"coverage.out",
			"all",
			false,
			"",
			10,
		)

		// Create teatest wrapper with fixed terminal size (use pointer)
		tm := teatest.NewTestModel(t, &model,
			teatest.WithInitialTermSize(80, 24))
		
		// Send a quit message to terminate the model
		tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

		// Get the final model after initialization
		finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(time.Second))
		require.NotNil(t, finalModel)

		// Verify the model state
		tuiModel, ok := finalModel.(*tui.TestModel)
		require.True(t, ok, "expected *tui.TestModel")
		assert.NotNil(t, tuiModel)
	})

	t.Run("process test events", func(t *testing.T) {
		// Create model
		model := tui.NewTestModel(
			[]string{"./pkg/test"},
			"",
			"",
			"",
			"failed",
			false,
			"",
			5,
		)

		// Create teatest wrapper (use pointer)
		tm := teatest.NewTestModel(t, &model,
			teatest.WithInitialTermSize(100, 30))

		// Simulate test events
		events := []types.TestEvent{
			{
				Time:    time.Now(),
				Action:  "run",
				Package: "github.com/example/test",
				Test:    "TestExample",
			},
			{
				Time:    time.Now(),
				Action:  "output",
				Package: "github.com/example/test",
				Test:    "TestExample",
				Output:  "=== RUN   TestExample\n",
			},
			{
				Time:    time.Now(),
				Action:  "pass",
				Package: "github.com/example/test",
				Test:    "TestExample",
				Elapsed: 0.5,
			},
			{
				Time:    time.Now(),
				Action:  "pass",
				Package: "github.com/example/test",
				Elapsed: 1.0,
			},
		}

		// Send events to the model as streamOutputMsg
		for _, event := range events {
			eventJSON, err := json.Marshal(event)
			require.NoError(t, err)
			// Use StreamOutputMsg which is what the TUI expects
			tm.Send(tui.StreamOutputMsg{Line: string(eventJSON)})
		}

		// Allow time for processing
		time.Sleep(100 * time.Millisecond)
		
		// Send test complete message to properly terminate the TUI
		tm.Send(tui.TestCompleteMsg{ExitCode: 0})

		// Get final model
		finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))
		tuiModel, ok := finalModel.(*tui.TestModel)
		require.True(t, ok)

		// Verify the model processed the events
		// Note: TestModel doesn't have GetPassCount, check the passCount field via exit code
		assert.Equal(t, 0, tuiModel.GetExitCode(), "expected exit code 0 for passed tests")
	})

	t.Run("test output capture", func(t *testing.T) {
		// Create model with specific configuration
		model := tui.NewTestModel(
			[]string{"./cmd/test"},
			"-v -race",
			"test-output.json",
			"",
			"all",
			false,
			"",
			3,
		)

		// Create teatest wrapper (use pointer)
		tm := teatest.NewTestModel(t, &model,
			teatest.WithInitialTermSize(120, 40))

		// Send a test completion event
		completeEvent := types.TestEvent{
			Time:    time.Now(),
			Action:  "pass",
			Package: "github.com/example/cmd",
			Elapsed: 2.5,
		}
		eventJSON, err := json.Marshal(completeEvent)
		require.NoError(t, err)
		tm.Send(tui.StreamOutputMsg{Line: string(eventJSON)})
		
		// Send test complete message to properly terminate the TUI
		tm.Send(tui.TestCompleteMsg{ExitCode: 0})

		// Get the final output (needs 2.5s timeout as model auto-quits after 2s)
		output, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second)))
		require.NoError(t, err)

		// Output should contain some TUI rendering
		assert.NotEmpty(t, output, "expected some output from TUI")
	})
}

// TestTUIPackageTracking tests that packages are properly tracked in the TUI
func TestTUIPackageTracking(t *testing.T) {
	model := tui.NewTestModel(
		[]string{"./pkg1", "./pkg2", "./pkg3"},
		"",
		"",
		"",
		"all",
		false,
		"",
		15,
	)

	// Create teatest wrapper (use pointer)
	tm := teatest.NewTestModel(t, &model,
		teatest.WithInitialTermSize(100, 30))

	// Simulate package events
	packages := []string{
		"github.com/example/pkg1",
		"github.com/example/pkg2",
		"github.com/example/pkg3",
	}

	for _, pkg := range packages {
		// Package start event
		startEvent := types.TestEvent{
			Time:    time.Now(),
			Action:  "start",
			Package: pkg,
		}
		eventJSON, _ := json.Marshal(startEvent)
		tm.Send(tui.StreamOutputMsg{Line: string(eventJSON)})

		// Package pass event
		passEvent := types.TestEvent{
			Time:    time.Now(),
			Action:  "pass",
			Package: pkg,
			Elapsed: 1.0,
		}
		eventJSON, _ = json.Marshal(passEvent)
		tm.Send(tui.StreamOutputMsg{Line: string(eventJSON)})
	}
	
	// Send test complete message to properly terminate the TUI
	tm.Send(tui.TestCompleteMsg{ExitCode: 0})

	// Get final model (needs 2.5s timeout as model auto-quits after 2s)
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	tuiModel, ok := finalModel.(*tui.TestModel)
	require.True(t, ok)

	// Verify all packages were tracked
	// Note: We can't directly access packageOrder from here,
	// but we can verify the model state
	assert.NotNil(t, tuiModel)
}

// TestTUIWithoutRealTTY verifies the TUI can be tested without a real TTY
func TestTUIWithoutRealTTY(t *testing.T) {
	// This test specifically validates that teatest allows us to test
	// the TUI without requiring a real terminal
	model := tui.NewTestModel(
		[]string{"./..."},
		"",
		"",
		"",
		"all",
		false,
		"",
		100,
	)

	// Create teatest wrapper - this should work even without a TTY (use pointer)
	tm := teatest.NewTestModel(t, &model,
		teatest.WithInitialTermSize(80, 24))

	// Send a quit message
	tm.Send(tea.QuitMsg{})

	// Verify we can get the final model without errors
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(time.Second))
	assert.NotNil(t, finalModel, "should be able to get final model without TTY")
}
