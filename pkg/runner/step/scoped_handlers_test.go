package step

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTitleHandlerSuppressedResolvesResult(t *testing.T) {
	handler, ok := Get("title")
	require.True(t, ok)
	for _, tc := range []struct {
		name, content, want string
		wantError           bool
	}{
		{"empty", "", "", false},
		{"template", "Atmos - {{ .steps.env.value }}", "Atmos - production", false},
		{"invalid template", "{{ .invalid", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := NewVariables()
			vars.Set("env", NewStepResult("production"))
			var output bytes.Buffer
			vars.OutputWriters = OutputWriters{Stdout: &output, Stderr: &output}
			result, err := handler.Execute(WithOutputSuppressed(context.Background()), &schema.WorkflowStep{Type: "title", Content: tc.content}, vars)
			if tc.wantError {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tc.want, result.Value)
			}
			assert.Empty(t, output.String())
		})
	}
}

func TestLogHandlerScopedLevelAndFields(t *testing.T) {
	handler, ok := Get("log")
	require.True(t, ok)
	for _, tc := range []struct {
		name, level, expected string
		fields                map[string]string
	}{
		{"default", "", "info Checks passed\n", nil},
		{"unknown defaults to info", "unknown", "info Checks passed\n", nil},
		{"normalized warning", "WaRnInG", "warn Checks passed\n", nil},
		{"trace", "trace", "trace Checks passed\n", nil},
		{"fields", "DEBUG", "debug Checks passed [region east]\n", map[string]string{"region": "{{ .steps.region.value }}"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := NewVariables()
			vars.Set("region", NewStepResult("east"))
			var output bytes.Buffer
			vars.OutputWriters.Stderr = &output
			result, err := handler.Execute(context.Background(), &schema.WorkflowStep{Type: "log", Content: "Checks passed", Level: tc.level, Fields: tc.fields}, vars)
			require.NoError(t, err)
			assert.Equal(t, "Checks passed", result.Value)
			assert.Equal(t, tc.expected, output.String())
		})
	}
}
