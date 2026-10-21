package step

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestApplyTapeOutputDirectiveSeedsEachExtension covers every supported Output
// extension and verifies an explicit step-level value always wins.
func TestApplyTapeOutputDirectiveSeedsEachExtension(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		field func(*schema.CastOutput) string
		set   func(*schema.CastOutput, string)
	}{
		{"cast", "demo.cast", func(o *schema.CastOutput) string { return o.Cast }, func(o *schema.CastOutput, v string) { o.Cast = v }},
		{"gif", "demo.gif", func(o *schema.CastOutput) string { return o.GIF }, func(o *schema.CastOutput, v string) { o.GIF = v }},
		{"mp4", "demo.mp4", func(o *schema.CastOutput) string { return o.MP4 }, func(o *schema.CastOutput, v string) { o.MP4 = v }},
		{"html", "demo.html", func(o *schema.CastOutput) string { return o.HTML }, func(o *schema.CastOutput, v string) { o.HTML = v }},
		{"ascii", "demo.ascii", func(o *schema.CastOutput) string { return o.ASCII }, func(o *schema.CastOutput, v string) { o.ASCII = v }},
		{"png", "demo.png", func(o *schema.CastOutput) string { return o.PNG }, func(o *schema.CastOutput, v string) { o.PNG = v }},
		{"jpg", "demo.jpg", func(o *schema.CastOutput) string { return o.JPG }, func(o *schema.CastOutput, v string) { o.JPG = v }},
		{"jpeg maps to JPG", "demo.jpeg", func(o *schema.CastOutput) string { return o.JPG }, func(o *schema.CastOutput, v string) { o.JPG = v }},
		{"uppercase extension", "DEMO.PNG", func(o *schema.CastOutput) string { return o.PNG }, func(o *schema.CastOutput, v string) { o.PNG = v }},
	}

	for _, tt := range tests {
		t.Run(tt.name+" seeds empty field", func(t *testing.T) {
			step := &schema.WorkflowStep{Name: "demo"}
			d := &asciicast.TapeDirective{Kind: asciicast.TapeOutput, Value: tt.path}
			require.NoError(t, applyTapeOutputDirective(step, d))
			require.NotNil(t, step.CastOutput)
			assert.Equal(t, tt.path, tt.field(step.CastOutput))
		})

		t.Run(tt.name+" explicit value wins", func(t *testing.T) {
			step := &schema.WorkflowStep{Name: "demo", CastOutput: &schema.CastOutput{}}
			tt.set(step.CastOutput, "explicit.out")
			d := &asciicast.TapeDirective{Kind: asciicast.TapeOutput, Value: tt.path}
			require.NoError(t, applyTapeOutputDirective(step, d))
			assert.Equal(t, "explicit.out", tt.field(step.CastOutput))
		})
	}
}

func TestExpandCastTapePropagatesUnsupportedOutputExtensionBeforeTranslation(t *testing.T) {
	// The error must surface from the settings pass, so no steps are produced.
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "steps", Tape: "Output demo.webm\nType \"echo hi\" Enter"}
	err := expandCastTape(step)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnsupportedTapeOutputExtension), "got %v", err)
	assert.Empty(t, step.Steps)
}

func TestExpandCastTapeRejectsInvalidMode(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "bogus", Tape: "Sleep 1s"}
	err := expandCastTape(step)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCastMode), "got %v", err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestTapeDirectiveKindName(t *testing.T) {
	tests := []struct {
		kind asciicast.TapeDirectiveKind
		want string
	}{
		{asciicast.TapeSleep, "Sleep"},
		{asciicast.TapeWait, "Wait"},
		{asciicast.TapeType, "directive"},
		{asciicast.TapeKey, "directive"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tapeDirectiveKindName(tt.kind))
	}
}

func TestParseTapeAssignmentsSkipsPairsWithoutEquals(t *testing.T) {
	got := parseTapeAssignments(`FOO BAR=baz QUOTED="quoted"`)
	assert.Equal(t, map[string]string{"BAR": "baz", "QUOTED": "quoted"}, got, "a bare word without '=' must be skipped")
}

func TestNewTapeTrackedStateSnapshotsExplicitEnv(t *testing.T) {
	step := &schema.WorkflowStep{
		WorkingDirectory: "work",
		Env:              map[string]string{"KEEP": "explicit"},
	}
	state := newTapeTrackedState(step)

	assert.True(t, state.explicitWorkingDirectory)
	assert.Equal(t, "work", state.workingDirectory)
	assert.True(t, state.explicitEnvKeys["KEEP"])
	assert.Equal(t, map[string]string{"KEEP": "explicit"}, state.env)

	// Mutating the tracked state must not leak back into the step's own env.
	state.env["NEW"] = "x"
	assert.NotContains(t, step.Env, "NEW")
}

func TestTapeTrackedStateExplicitEnvAndCwdWin(t *testing.T) {
	step := &schema.WorkflowStep{
		WorkingDirectory: "fixed",
		Env:              map[string]string{"KEEP": "explicit"},
	}
	state := newTapeTrackedState(step)

	state.apply(tapeSetupCommand{Kind: "export", Env: map[string]string{"KEEP": "tape", "OTHER": "tape"}})
	assert.Equal(t, "explicit", state.env["KEEP"], "explicit env key must survive a tape export")
	assert.Equal(t, "tape", state.env["OTHER"])

	state.apply(tapeSetupCommand{Kind: "unset", Env: map[string]string{"KEEP": "", "OTHER": ""}})
	assert.Equal(t, "explicit", state.env["KEEP"], "explicit env key must survive a tape unset")
	assert.NotContains(t, state.env, "OTHER")

	state.apply(tapeSetupCommand{Kind: "cd", Dir: "elsewhere"})
	assert.Equal(t, "fixed", state.workingDirectory, "explicit working_directory must survive a tape cd")
}

func TestExpandCastTapeStepsModeIgnoresSettingsDirectives(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
		Tape: "Set Width 80\nOutput demo.gif\nRequire atmos\nType \"echo hi\" Enter",
	}
	require.NoError(t, expandCastTape(step))

	want := []schema.WorkflowStep{
		{Type: schema.TaskTypeSimulate, Mode: "typed", Text: "echo hi"},
		{Type: schema.TaskTypeShell, Command: "echo hi"},
	}
	assertWorkflowStepsEqual(t, want, step.Steps)
	assert.Equal(t, 80, step.Width)
	require.NotNil(t, step.CastOutput)
	assert.Equal(t, "demo.gif", step.CastOutput.GIF)
	assert.Equal(t, []string{"atmos"}, step.Tools)
}

func TestExpandCastTapeSessionModeUnterminatedHideFlushesBufferedActions(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: "Hide\nType \"docker compose up -d\" Enter",
	}
	require.NoError(t, expandCastTape(step))

	want := []schema.WorkflowStep{
		{Type: "hide"},
		{Type: "write", Text: "docker compose up -d"},
		{Type: "key", Key: "enter"},
	}
	assertWorkflowStepsEqual(t, want, step.Steps)
}
