package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCastPlaybackValidation(t *testing.T) {
	for _, mode := range []string{"steps", "session"} {
		t.Run(mode, func(t *testing.T) {
			for _, duration := range []string{"", "no", "0s", "-1s"} {
				cast := &schema.WorkflowStep{Mode: mode, Steps: []schema.WorkflowStep{{Type: "playback", Duration: duration, Steps: []schema.WorkflowStep{{Type: "pause", Duration: "1ms"}}}}}
				require.ErrorIs(t, (&CastHandler{}).Validate(cast), errUtils.ErrInvalidCastPlayback)
			}
			cast := &schema.WorkflowStep{Mode: mode, Steps: []schema.WorkflowStep{{Type: "playback", Duration: "4s"}}}
			require.ErrorIs(t, (&CastHandler{}).Validate(cast), errUtils.ErrInvalidCastPlayback)
			cast.Steps[0].Steps = []schema.WorkflowStep{{Type: "playback", Duration: "1s", Steps: []schema.WorkflowStep{{Type: "pause", Duration: "1ms"}}}}
			require.ErrorIs(t, (&CastHandler{}).Validate(cast), errUtils.ErrInvalidCastPlayback)
			cast.Steps[0].Steps = []schema.WorkflowStep{{Type: "simulate", Text: "hello"}}
			require.NoError(t, (&CastHandler{}).Validate(cast))
		})
	}
	cast := &schema.WorkflowStep{Mode: "session", Steps: []schema.WorkflowStep{{Type: "playback", Duration: "4s", Steps: []schema.WorkflowStep{{Type: "wait"}}}}}
	require.ErrorIs(t, (&CastHandler{}).Validate(cast), ErrWaitActionRequiresTextOrRegex)
}

func TestCastPlaybackSessionEndToEnd(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	shell, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(sessionShellHelperEnv, "1")
	path := filepath.Join(t.TempDir(), "session.cast")
	cast := &schema.WorkflowStep{Type: "cast", Mode: "session", Shell: shell, CastOutput: &schema.CastOutput{Cast: path}, Steps: []schema.WorkflowStep{
		{Type: "write", Text: "printf ready", Rate: "0"},
		{Type: "playback", Duration: "4s", Steps: []schema.WorkflowStep{
			{Type: "key", Key: "enter"},
			{Type: "wait", Regex: sessionReadyOutputRegex, Timeout: sessionReadyWaitTimeout},
		}},
		{Type: "pause", Duration: "100ms"},
	}}
	require.NoError(t, (&CastHandler{}).Validate(cast))
	_, err = (&CastHandler{}).Execute(context.Background(), cast, NewVariables())
	require.NoError(t, err)
	_, events, err := asciicast.ReadEvents(path)
	require.NoError(t, err)
	var text strings.Builder
	var endpoint float64
	for _, event := range events {
		if event.Data == "" {
			endpoint = event.Time
		}
		text.WriteString(event.Data)
	}
	require.Contains(t, text.String(), "ready")
	require.GreaterOrEqual(t, endpoint, 4.0)
	require.Less(t, endpoint, 4.0+sessionReadyWaitTimeoutDuration.Seconds())
}

func TestCastPlaybackStepsEndToEnd(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	path := filepath.Join(t.TempDir(), "steps.cast")
	cast := &schema.WorkflowStep{Type: "cast", CastOutput: &schema.CastOutput{Cast: path}, Steps: []schema.WorkflowStep{
		{Type: "playback", Duration: "4s", Steps: []schema.WorkflowStep{
			{Type: "simulate", Text: "frame", Rate: "0", Duration: "0", Interval: "0"},
		}},
		{Type: "simulate", Text: "after", Rate: "0", Duration: "0", Interval: "0"},
	}}
	_, err := (&CastHandler{}).Execute(context.Background(), cast, NewVariables())
	require.NoError(t, err)
	_, events, err := asciicast.ReadEvents(path)
	require.NoError(t, err)
	var text strings.Builder
	var endpoint float64
	for _, event := range events {
		if event.Data == "" {
			endpoint = event.Time
		}
		text.WriteString(event.Data)
	}
	require.Contains(t, text.String(), "frame")
	require.Contains(t, text.String(), "after")
	require.InDelta(t, 4.0, endpoint, 0.1)
	require.GreaterOrEqual(t, events[len(events)-1].Time, endpoint)
}

func TestCastPlaybackFinalPromptCursor(t *testing.T) {
	for _, tc := range []struct {
		name    string
		grouped bool
		cursor  bool
	}{
		{name: "direct cursor", cursor: true},
		{name: "playback cursor", grouped: true, cursor: true},
		{name: "playback without cursor", grouped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, iolib.Initialize())
			path := filepath.Join(t.TempDir(), "cursor.cast")
			child := schema.WorkflowStep{Type: "simulate", Text: "frame", Cursor: tc.cursor, Rate: "0", Duration: "0", Interval: "0"}
			if tc.grouped {
				child = schema.WorkflowStep{Type: "playback", Duration: "1s", Steps: []schema.WorkflowStep{child}}
			}
			cast := &schema.WorkflowStep{Type: "cast", CastOutput: &schema.CastOutput{Cast: path}, Steps: []schema.WorkflowStep{child}}
			_, err := (&CastHandler{}).Execute(context.Background(), cast, NewVariables())
			require.NoError(t, err)
			_, events, err := asciicast.ReadEvents(path)
			require.NoError(t, err)
			require.NotEmpty(t, events)
			finalPrompt := events[len(events)-1].Data
			require.Contains(t, ansi.Strip(finalPrompt), "> ")
			require.Equal(t, tc.cursor, strings.HasSuffix(finalPrompt, castCursorShow), "final prompt must preserve cursor visibility")
		})
	}
}

func TestCastPlaybackFailurePreservesPreviousRecording(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	shell, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(sessionShellHelperEnv, "1")
	path := filepath.Join(t.TempDir(), "previous.cast")
	require.NoError(t, os.WriteFile(path, []byte("previous successful cast"), 0o600))
	cast := &schema.WorkflowStep{Type: "cast", Mode: "session", Shell: shell, CastOutput: &schema.CastOutput{Cast: path}, Steps: []schema.WorkflowStep{
		{Type: "playback", Duration: "4s", Steps: []schema.WorkflowStep{{Type: "wait", Text: "missing", Timeout: "1ms"}}},
	}}
	_, err = (&CastHandler{}).Execute(context.Background(), cast, NewVariables())
	require.ErrorIs(t, err, asciicast.ErrWaitTimeout)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "previous successful cast", string(contents))
}

func TestCastPlaybackPreservesPromptAcrossGroupBoundaries(t *testing.T) {
	for _, groupSession := range []bool{true, false} {
		name := "narration-in-group"
		if groupSession {
			name = "session-in-group"
		}
		t.Run(name, func(t *testing.T) {
			require.NoError(t, iolib.Initialize())
			shell, err := os.Executable()
			require.NoError(t, err)
			t.Setenv(sessionShellHelperEnv, "1")
			steps := []schema.WorkflowStep{
				{Type: "session", Shell: shell, Steps: []schema.WorkflowStep{
					{Type: "write", Text: "printf ready", Rate: "0"},
					{Type: "key", Key: "enter"},
					{Type: "wait", Regex: sessionReadyOutputRegex, Timeout: sessionReadyWaitTimeout},
				}},
				{Type: "simulate", Text: "# narration", Rate: "0", Duration: "0", Interval: "0"},
			}
			index := 1
			if groupSession {
				index = 0
			}
			steps[index] = schema.WorkflowStep{Type: "playback", Duration: "1s", Steps: []schema.WorkflowStep{steps[index]}}
			path := filepath.Join(t.TempDir(), "prompts.cast")
			_, err = (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
				Type: "cast", CastOutput: &schema.CastOutput{Cast: path}, Steps: steps,
			}, NewVariables())
			require.NoError(t, err)
			contents, err := os.ReadFile(path)
			require.NoError(t, err)
			text := ansi.Strip(castOutputText(t, contents))
			require.Contains(t, text, "# narration")
			require.NotContains(t, text, "> >")
		})
	}
}
