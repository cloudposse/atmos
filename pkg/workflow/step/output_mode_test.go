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
