package step

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestCastValidatePropagatesTapeExpansionError(t *testing.T) {
	err := (&CastHandler{}).Validate(&schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Output demo.webm",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnsupportedTapeOutputExtension), "got %v", err)
}

func TestCastValidateReturnsSourceLoadError(t *testing.T) {
	err := (&CastHandler{}).Validate(&schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		WorkingDirectory: t.TempDir(), Source: "missing.tape",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrTapeSourceNotFound), "got %v", err)
}

func TestCastValidateSessionAcceptsHideAndShowActions(t *testing.T) {
	err := (&CastHandler{}).Validate(&schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Steps: []schema.WorkflowStep{{Type: "hide"}, {Type: "write", Text: "echo hi"}, {Type: "show"}},
	})
	require.NoError(t, err)
}

func TestCastValidateScreenshotPathRequirement(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		steps   []schema.WorkflowStep
		wantErr error
	}{
		{
			name:    "steps mode screenshot without path",
			mode:    "steps",
			steps:   []schema.WorkflowStep{{Type: schema.TaskTypeScreenshot}},
			wantErr: ErrScreenshotActionRequiresPath,
		},
		{
			name:  "steps mode screenshot with path",
			mode:  "steps",
			steps: []schema.WorkflowStep{{Type: schema.TaskTypeScreenshot, Path: "shot.png"}},
		},
		{
			name:    "session mode screenshot without path",
			mode:    "session",
			steps:   []schema.WorkflowStep{{Type: schema.TaskTypeScreenshot}},
			wantErr: ErrScreenshotActionRequiresPath,
		},
		{
			name:  "session mode screenshot with path",
			mode:  "session",
			steps: []schema.WorkflowStep{{Type: schema.TaskTypeScreenshot, Path: "shot.png"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&CastHandler{}).Validate(&schema.WorkflowStep{
				Name: "demo", Type: schema.TaskTypeCast, Mode: tt.mode, Steps: tt.steps,
			})
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr), "got %v", err)
		})
	}
}

func TestCastValidateSessionSimulateAction(t *testing.T) {
	tests := []struct {
		name    string
		action  schema.WorkflowStep
		wantErr error
	}{
		{
			name:    "typed simulate without text",
			action:  schema.WorkflowStep{Type: schema.TaskTypeSimulate, Mode: "typed"},
			wantErr: ErrSimulateTypedRequiresText,
		},
		{
			name:   "typed simulate with text",
			action: schema.WorkflowStep{Type: schema.TaskTypeSimulate, Mode: "typed", Text: "atmos version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&CastHandler{}).Validate(&schema.WorkflowStep{
				Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Steps: []schema.WorkflowStep{tt.action},
			})
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr), "got %v", err)
		})
	}
}

// TestCastHandlerStepsScreenshotChildRecordsMarker proves a screenshot child
// in mode: steps emits a named "m" checkpoint into the recording (consumed
// later by the marker screenshot renderer) instead of being executed as a
// regular step.
func TestCastHandlerStepsScreenshotChildRecordsMarker(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	ui.InitFormatter(iolib.GetContext())
	dir := t.TempDir()
	castPath := filepath.Join(dir, "demo.cast")
	shotPath := filepath.Join(dir, "after-install.png")

	_, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name:       "demo",
		Type:       schema.TaskTypeCast,
		Output:     string(OutputModeNone),
		CastOutput: &schema.CastOutput{Cast: castPath},
		Steps: []schema.WorkflowStep{
			{Type: schema.TaskTypeScreenshot, Path: shotPath},
		},
	}, NewVariables())
	require.NoError(t, err)

	content, err := os.ReadFile(castPath)
	require.NoError(t, err)

	var markers int
	for _, line := range strings.Split(string(content), "\n") {
		if strings.Contains(line, `"m"`) && strings.Contains(line, "after-install.png") {
			markers++
		}
	}
	assert.Equal(t, 1, markers, "expected exactly one marker event naming the screenshot path:\n%s", content)

	// The marker is what drives screenshot rendering after the recording closes.
	info, statErr := os.Stat(shotPath)
	require.NoError(t, statErr, "marker should have been rendered to a PNG")
	assert.Positive(t, info.Size())
}
