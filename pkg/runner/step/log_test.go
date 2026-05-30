package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// LogHandler registration and basic validation are tested in output_handlers_test.go.
// This file tests Execute and helper methods.

func TestLogHandler_GetLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected log.Level
	}{
		{"trace", "trace", log.TraceLevel},
		{"debug", "debug", log.DebugLevel},
		{"info", "info", log.InfoLevel},
		{"warn", "warn", log.WarnLevel},
		{"warning", "warning", log.WarnLevel},
		{"error", "error", log.ErrorLevel},
		{"empty defaults to info", "", log.InfoLevel},
		{"unknown defaults to info", "unknown", log.InfoLevel},
		{"uppercase", "DEBUG", log.DebugLevel},
		{"mixed case", "WaRn", log.WarnLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := getLogLevel(tt.level)
			assert.Equal(t, tt.expected, level)
		})
	}
}

func TestLogHandler_BuildKeyvals(t *testing.T) {
	handler, ok := Get("log")
	require.True(t, ok)
	logHandler := handler.(*LogHandler)

	t.Run("empty fields", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:   "test",
			Fields: nil,
		}
		vars := NewVariables()

		keyvals := logHandler.buildKeyvals(step, vars)
		assert.Nil(t, keyvals)
	})

	t.Run("static fields", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Fields: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}
		vars := NewVariables()

		keyvals := logHandler.buildKeyvals(step, vars)
		assert.NotNil(t, keyvals)
		// Should have 4 elements (2 key-value pairs).
		assert.Len(t, keyvals, 4)
	})

	t.Run("template fields", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Fields: map[string]string{
				"env": "{{ .steps.env.value }}",
			},
		}
		vars := NewVariables()
		vars.Set("env", NewStepResult("production"))

		keyvals := logHandler.buildKeyvals(step, vars)
		assert.NotNil(t, keyvals)
		// Should contain "env" and "production".
		found := false
		for i := 0; i < len(keyvals); i += 2 {
			if keyvals[i] == "env" && keyvals[i+1] == "production" {
				found = true
				break
			}
		}
		assert.True(t, found, "should contain env=production")
	})

	t.Run("invalid template uses original value", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Fields: map[string]string{
				"bad": "{{ .steps.invalid.value",
			},
		}
		vars := NewVariables()

		keyvals := logHandler.buildKeyvals(step, vars)
		assert.NotNil(t, keyvals)
		// Should fallback to original value.
		found := false
		for i := 0; i < len(keyvals); i += 2 {
			if keyvals[i] == "bad" && keyvals[i+1] == "{{ .steps.invalid.value" {
				found = true
				break
			}
		}
		assert.True(t, found, "should fallback to original value on template error")
	})
}

func TestLogHandlerValidation(t *testing.T) {
	handler, ok := Get("log")
	require.True(t, ok)

	t.Run("valid with content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "log",
			Content: "Log message",
		}
		err := handler.Validate(step)
		assert.NoError(t, err)
	})

	t.Run("missing content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "log",
		}
		err := handler.Validate(step)
		assert.Error(t, err)
	})

	t.Run("with level and fields", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "log",
			Content: "Log message",
			Level:   "debug",
			Fields: map[string]string{
				"component": "vpc",
			},
		}
		err := handler.Validate(step)
		assert.NoError(t, err)
	})
}

func TestLogHandlerExecution(t *testing.T) {
	handler, ok := Get("log")
	require.True(t, ok)

	t.Run("simple log message", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_log",
			Type:    "log",
			Content: "Test log message",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Test log message", result.Value)
	})

	t.Run("log with template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_log",
			Type:    "log",
			Content: "Deploying {{ .steps.component.value }}",
		}
		vars := NewVariables()
		vars.Set("component", NewStepResult("vpc"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Deploying vpc", result.Value)
	})

	t.Run("log with different levels", func(t *testing.T) {
		levels := []string{"trace", "debug", "info", "warn", "error"}
		for _, level := range levels {
			step := &schema.WorkflowStep{
				Name:    "test_log",
				Type:    "log",
				Content: "Log at " + level,
				Level:   level,
			}
			vars := NewVariables()

			result, err := handler.Execute(context.Background(), step, vars)
			require.NoError(t, err)
			assert.Equal(t, "Log at "+level, result.Value)
		}
	})

	t.Run("log with fields", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_log",
			Type:    "log",
			Content: "Deployment complete",
			Fields: map[string]string{
				"component": "vpc",
				"env":       "production",
			},
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Deployment complete", result.Value)
	})

	t.Run("log with invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_log",
			Type:    "log",
			Content: "Invalid {{ .steps.missing.value",
		}
		vars := NewVariables()

		_, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
	})
}
