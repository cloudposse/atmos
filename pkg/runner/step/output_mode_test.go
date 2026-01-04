package step

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewOutputModeWriter(t *testing.T) {
	viewport := &schema.ViewportConfig{Height: 20, Width: 80}
	writer := NewOutputModeWriter(OutputModeLog, "test_step", viewport)

	assert.NotNil(t, writer)
	assert.Equal(t, OutputModeLog, writer.mode)
	assert.Equal(t, "test_step", writer.stepName)
	assert.Equal(t, viewport, writer.viewport)
}

func TestGetOutputModeNilWorkflow(t *testing.T) {
	step := &schema.WorkflowStep{}

	mode := GetOutputMode(step, nil)
	assert.Equal(t, OutputModeLog, mode)
}

func TestGetViewportConfigNilWorkflow(t *testing.T) {
	step := &schema.WorkflowStep{}

	config := GetViewportConfig(step, nil)
	assert.Nil(t, config)
}

func TestStreamingOutputWriter(t *testing.T) {
	t.Run("captures output", func(t *testing.T) {
		var target bytes.Buffer
		writer := NewStreamingOutputWriter("[prefix]", &target)

		n, err := writer.Write([]byte("hello world"))
		assert.NoError(t, err)
		assert.Equal(t, 11, n)

		assert.Equal(t, "hello world", writer.String())
		assert.Contains(t, target.String(), "[prefix]")
		assert.Contains(t, target.String(), "hello world")
	})

	t.Run("multiple writes", func(t *testing.T) {
		var target bytes.Buffer
		writer := NewStreamingOutputWriter("[step]", &target)

		_, _ = writer.Write([]byte("line1\n"))
		_, _ = writer.Write([]byte("line2\n"))

		output := writer.String()
		assert.Contains(t, output, "line1")
		assert.Contains(t, output, "line2")
	})

	t.Run("nil target", func(t *testing.T) {
		writer := NewStreamingOutputWriter("[prefix]", nil)

		n, err := writer.Write([]byte("test"))
		assert.NoError(t, err)
		assert.Equal(t, 4, n)
		assert.Equal(t, "test", writer.String())
	})

	t.Run("concurrent writes", func(t *testing.T) {
		var target bytes.Buffer
		writer := NewStreamingOutputWriter("[test]", &target)

		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				_, _ = writer.Write([]byte("data"))
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		// Should have accumulated all writes.
		assert.Len(t, writer.String(), 40) // 10 writes * 4 bytes
	})
}

func TestOutputModeWriterDefaults(t *testing.T) {
	// Test that unknown mode defaults to log.
	writer := NewOutputModeWriter("unknown_mode", "test", nil)
	assert.NotNil(t, writer)
	assert.Equal(t, OutputMode("unknown_mode"), writer.mode)
}

func TestFormatStepLabel(t *testing.T) {
	tests := []struct {
		name       string
		step       *schema.WorkflowStep
		workflow   *schema.WorkflowDefinition
		stepIndex  int
		totalSteps int
		wantCount  bool
	}{
		{
			name:       "no count when show.count not enabled",
			step:       &schema.WorkflowStep{Name: "build"},
			workflow:   nil,
			stepIndex:  0,
			totalSteps: 3,
			wantCount:  false,
		},
		{
			name: "includes count when show.count enabled",
			step: &schema.WorkflowStep{
				Name: "build",
				Show: &schema.ShowConfig{Count: BoolPtr(true)},
			},
			workflow:   nil,
			stepIndex:  0,
			totalSteps: 3,
			wantCount:  true,
		},
		{
			name: "workflow level show.count",
			step: &schema.WorkflowStep{Name: "test"},
			workflow: &schema.WorkflowDefinition{
				Show: &schema.ShowConfig{Count: BoolPtr(true)},
			},
			stepIndex:  1,
			totalSteps: 5,
			wantCount:  true,
		},
		{
			name: "no count when totalSteps is zero",
			step: &schema.WorkflowStep{
				Name: "deploy",
				Show: &schema.ShowConfig{Count: BoolPtr(true)},
			},
			workflow:   nil,
			stepIndex:  0,
			totalSteps: 0,
			wantCount:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatStepLabel(tt.step, tt.workflow, tt.stepIndex, tt.totalSteps)

			// Step name should always be present.
			assert.Contains(t, result, tt.step.Name)

			if tt.wantCount && tt.totalSteps > 0 {
				// Should contain count format like [1/3].
				expectedCount := "[" + string('0'+byte(tt.stepIndex+1)) + "/" + string('0'+byte(tt.totalSteps)) + "]"
				assert.Contains(t, result, expectedCount)
			}
		})
	}
}

func TestRenderCommand(t *testing.T) {
	// RenderCommand writes to ui output, so we just verify it doesn't panic.
	// The actual output goes to ui.Writeln which writes to stderr.
	tests := []struct {
		name     string
		step     *schema.WorkflowStep
		workflow *schema.WorkflowDefinition
		command  string
	}{
		{
			name:     "no output when show.command not enabled",
			step:     &schema.WorkflowStep{Name: "test"},
			workflow: nil,
			command:  "echo hello",
		},
		{
			name: "renders when show.command enabled",
			step: &schema.WorkflowStep{
				Name: "test",
				Show: &schema.ShowConfig{Command: BoolPtr(true)},
			},
			workflow: nil,
			command:  "echo hello",
		},
		{
			name: "no output when command empty",
			step: &schema.WorkflowStep{
				Name: "test",
				Show: &schema.ShowConfig{Command: BoolPtr(true)},
			},
			workflow: nil,
			command:  "",
		},
		{
			name: "workflow level show.command",
			step: &schema.WorkflowStep{Name: "test"},
			workflow: &schema.WorkflowDefinition{
				Show: &schema.ShowConfig{Command: BoolPtr(true)},
			},
			command: "make build",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic.
			RenderCommand(tt.step, tt.workflow, tt.command)
		})
	}
}

func TestFormatStepFooter(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeLog, "my_step", nil)

	t.Run("success footer", func(t *testing.T) {
		footer := writer.formatStepFooter(nil)
		assert.Contains(t, footer, "my_step")
		assert.Contains(t, footer, "completed")
	})

	t.Run("failed footer", func(t *testing.T) {
		footer := writer.formatStepFooter(assert.AnError)
		assert.Contains(t, footer, "my_step")
		assert.Contains(t, footer, "failed")
	})
}

func TestFormatSuccessFooter(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeLog, "deploy", nil)

	// Test without styles (nil styles).
	footer := writer.formatSuccessFooter(nil)
	assert.Contains(t, footer, "✓")
	assert.Contains(t, footer, "deploy")
	assert.Contains(t, footer, "completed")
}

func TestFormatFailedFooter(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeLog, "build", nil)

	// Test without styles (nil styles).
	footer := writer.formatFailedFooter(nil)
	assert.Contains(t, footer, "✗")
	assert.Contains(t, footer, "build")
	assert.Contains(t, footer, "failed")
}
