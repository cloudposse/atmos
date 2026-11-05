package step

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: a rename of a field these tests rely on fails the build.
var (
	_ = schema.WorkflowStep{ShellInit: "clean", LiteralFields: []string{"text"}}
	_ = schema.CastOutput{TapeOutputFormats: map[string]bool{"gif": true}}
	_ = asciicast.TapeDirective{Speed: "50ms", Timeout: "10s"}
	_ = asciicast.SessionOptions{ShellInit: "clean"}
	_ = asciicast.RenderOptions{Overwrite: true}
)

func TestTapeGeometryConvertsVHSPixelsToCells(t *testing.T) {
	tests := []struct {
		name       string
		tape       string
		wantWidth  int
		wantHeight int
	}{
		{name: "vhs defaults", tape: "Set Width 1200\nSet Height 600", wantWidth: 81, wantHeight: 21},
		{name: "legacy demo tape", tape: "Set FontSize 14\nSet Padding 10\nSet Width 1400\nSet Height 800", wantWidth: 164, wantHeight: 55},
		{name: "order independent", tape: "Set Width 1400\nSet Height 800\nSet FontSize 14\nSet Padding 10", wantWidth: 164, wantHeight: 55},
		{name: "only width", tape: "Set Width 1200", wantWidth: 81, wantHeight: 0},
		{name: "only height", tape: "Set Height 600", wantWidth: 0, wantHeight: 21},
		{name: "letter spacing and line height", tape: "Set FontSize 10\nSet Padding 0\nSet LetterSpacing 1\nSet LineHeight 2\nSet Width 700\nSet Height 400", wantWidth: 100, wantHeight: 20},
		{name: "clamped to a minimum", tape: "Set Width 100\nSet Height 100", wantWidth: tapeMinColumns, wantHeight: tapeMinRows},
		{name: "unparseable number is ignored", tape: "Set FontSize big\nSet Width 1200\nSet Height 600", wantWidth: 81, wantHeight: 21},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: tt.tape + "\nSleep 1ms"}
			require.NoError(t, expandCastTape(step))
			assert.Equal(t, tt.wantWidth, step.Width)
			assert.Equal(t, tt.wantHeight, step.Height)
		})
	}
}

func TestTapeGeometryExplicitStepSizeWins(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Width: 100, Height: 30,
		Tape: "Set Width 1200\nSet Height 600\nSleep 1ms",
	}
	require.NoError(t, expandCastTape(step))
	assert.Equal(t, 100, step.Width)
	assert.Equal(t, 30, step.Height)
}

func TestTapeGeometryWarnsOnNonNumericAndDoesNotWarnAboutFontKeys(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Set FontSize 14\nSet Padding oops\nSet Width 1200\nSleep 1ms"}
	out := captureTapeLogOutput(t, func() { require.NoError(t, expandCastTape(step)) })
	assert.Contains(t, out, "non-numeric")
	assert.Contains(t, out, "Padding")
	assert.NotContains(t, out, "FontSize", "FontSize feeds the size conversion, so it is no longer reported as skipped")
}

// TestTapeGeometryWarnsWhenClamped verifies a cell-count-looking value (a
// common mistake, since VHS sizes are pixels) warns rather than clamping
// silently, and that an in-range value does not warn.
func TestTapeGeometryWarnsWhenClamped(t *testing.T) {
	clamped := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Set Width 80\nSleep 1ms"}
	out := captureTapeLogOutput(t, func() { require.NoError(t, expandCastTape(clamped)) })
	assert.Equal(t, tapeMinColumns, clamped.Width)
	assert.Contains(t, out, "clamping")
	assert.Contains(t, out, "Width")

	inRange := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Set Width 1200\nSleep 1ms"}
	out = captureTapeLogOutput(t, func() { require.NoError(t, expandCastTape(inRange)) })
	assert.NotContains(t, out, "clamping")
}

func TestTapeGeometryNonPositiveCellSizeIsIgnored(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Set FontSize 0\nSet Width 1200\nSet Height 600\nSleep 1ms"}
	out := captureTapeLogOutput(t, func() { require.NoError(t, expandCastTape(step)) })
	assert.Zero(t, step.Width)
	assert.Zero(t, step.Height)
	assert.Contains(t, out, "cannot convert pixel size")
}

func TestTapeTextIsLiteralInStepsMode(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
		Tape: `Type "echo '{{.Names}}'" Enter`,
	}
	require.NoError(t, expandCastTape(step))
	require.Len(t, step.Steps, 2)

	vars := NewVariables()
	simulate, shell := &step.Steps[0], &step.Steps[1]
	assert.True(t, simulate.IsLiteral("text"))
	assert.True(t, shell.IsLiteral("command"))

	text, err := vars.ResolveStepField(simulate, "text", simulate.Text)
	require.NoError(t, err)
	assert.Equal(t, "echo '{{.Names}}'", text)

	command, err := (&ShellHandler{}).ResolveCommand(context.Background(), shell, vars)
	require.NoError(t, err)
	assert.Equal(t, "echo '{{.Names}}'", command)
}

func TestHandWrittenStepsStillRenderTemplates(t *testing.T) {
	vars := NewVariables()
	simulate := &schema.WorkflowStep{Type: schema.TaskTypeSimulate, Mode: "typed", Text: `{{ "hi" }}`}
	text, err := vars.ResolveStepField(simulate, "text", simulate.Text)
	require.NoError(t, err)
	assert.Equal(t, "hi", text)

	shell := &schema.WorkflowStep{Type: schema.TaskTypeShell, Command: `echo {{ "hi" }}`}
	command, err := (&ShellHandler{}).ResolveCommand(context.Background(), shell, vars)
	require.NoError(t, err)
	assert.Equal(t, "echo hi", command)
}

func TestTapeLiftedEnvAndCwdAreLiteral(t *testing.T) {
	t.Run("steps mode children", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
			Tape: "Type \"export MSG='{{.X}} hello'\" Enter\nType \"cd sub\" Enter\nType \"echo hi\" Enter",
		}
		require.NoError(t, expandCastTape(step))
		shell := step.Steps[len(step.Steps)-1]
		require.Equal(t, schema.TaskTypeShell, shell.Type)
		assert.True(t, shell.IsLiteralEnv("MSG"))
		assert.True(t, shell.IsLiteral("working_directory"))
		env, err := NewVariables().ResolveStepEnvMap(&shell, shell.Env)
		require.NoError(t, err)
		assert.Equal(t, "{{.X}} hello", env["MSG"])
	})

	t.Run("session mode step", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
			Tape: "Hide\nType \"export MSG='{{.X}} hello'\" Enter\nShow\nType \"echo hi\" Enter",
		}
		require.NoError(t, expandCastTape(step))
		assert.True(t, step.IsLiteralEnv("MSG"))
		env, err := castRecorderEnv(step, NewVariables())
		require.NoError(t, err)
		assert.Equal(t, "{{.X}} hello", env["MSG"])
	})
}

func TestParseTapeSetupCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ATMOS_TAPE_TEST_BASE", "base")

	tests := []struct {
		name     string
		text     string
		wantOK   bool
		wantKind string
		wantEnv  map[string]string
		wantDir  string
	}{
		{name: "export with variable expansion", text: "export P=$ATMOS_TAPE_TEST_BASE:/opt/extra", wantOK: true, wantKind: tapeSetupEnv, wantEnv: map[string]string{"P": "base:/opt/extra"}},
		{name: "export quoted value keeps the space", text: `export MSG="hello world"`, wantOK: true, wantKind: tapeSetupEnv, wantEnv: map[string]string{"MSG": "hello world"}},
		{name: "export single quoted dollar stays literal", text: `export MSG='$HOME'`, wantOK: true, wantKind: tapeSetupEnv, wantEnv: map[string]string{"MSG": "$HOME"}},
		{name: "export sees earlier assignment", text: "export A=1 B=$A", wantOK: true, wantKind: tapeSetupEnv, wantEnv: map[string]string{"A": "1", "B": "1"}},
		{name: "export append", text: "export ATMOS_TAPE_TEST_BASE+=:more", wantOK: true, wantKind: tapeSetupEnv, wantEnv: map[string]string{"ATMOS_TAPE_TEST_BASE": "base:more"}},
		{name: "export empty value", text: "export EMPTY=", wantOK: true, wantKind: tapeSetupEnv, wantEnv: map[string]string{"EMPTY": ""}},
		{name: "unset", text: "unset FOO BAR", wantOK: true, wantKind: tapeSetupUnset, wantEnv: map[string]string{"FOO": "", "BAR": ""}},
		{name: "clear", text: "clear", wantOK: true, wantKind: tapeSetupClear},
		{name: "cd relative", text: "cd sub", wantOK: true, wantKind: tapeSetupCd, wantDir: "sub"},
		{name: "cd quoted", text: `cd "my dir"`, wantOK: true, wantKind: tapeSetupCd, wantDir: "my dir"},
		{name: "cd tilde", text: "cd ~", wantOK: true, wantKind: tapeSetupCd, wantDir: home},
		{name: "cd tilde subdirectory", text: "cd ~/x", wantOK: true, wantKind: tapeSetupCd, wantDir: home + "/x"},
		{name: "bare cd goes home", text: "cd", wantOK: true, wantKind: tapeSetupCd, wantDir: home},
		{name: "trailing comment", text: "cd sub # go there", wantOK: true, wantKind: tapeSetupCd, wantDir: "sub"},

		{name: "compound cd", text: "cd sub && pwd && echo X"},
		{name: "list with semicolon", text: "cd a; cd b"},
		{name: "pipe", text: "export A=1 | cat"},
		{name: "redirect", text: "cd sub > out.txt"},
		{name: "background", text: "cd sub &"},
		{name: "subshell", text: "(cd sub)"},
		{name: "negated", text: "! cd sub"},
		{name: "command substitution in cd", text: `cd "$(git rev-parse --show-toplevel)"`},
		{name: "command substitution in export", text: "export X=$(date)"},
		{name: "backtick substitution", text: "export X=`date`"},
		{name: "cd dash", text: "cd -"},
		{name: "cd option", text: "cd -P sub"},
		{name: "cd two operands", text: "cd a b"},
		{name: "assignment prefix", text: "FOO=1 cd sub"},
		{name: "export without operands", text: "export"},
		{name: "export flag", text: "export -n FOO"},
		{name: "export array", text: "export A=(1 2)"},
		{name: "unset function flag", text: "unset -f foo"},
		{name: "clear with flag", text: "clear -x"},
		{name: "plain command", text: "echo hi"},
		{name: "empty text", text: ""},
		{name: "unparsable text", text: "cd 'unterminated"},
		{name: "two lines", text: "cd a\ncd b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newTapeTrackedState(&schema.WorkflowStep{})
			got, ok := parseTapeSetupCommand(tt.text, state)
			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			assert.Equal(t, tt.wantKind, got.Kind)
			if tt.wantEnv != nil {
				assert.Equal(t, tt.wantEnv, got.Env)
			}
			assert.Equal(t, tt.wantDir, got.Dir)
		})
	}
}

func TestParseTapeSetupCommandUsesTrackedEnv(t *testing.T) {
	state := newTapeTrackedState(&schema.WorkflowStep{Env: map[string]string{"TRACKED": "from-step"}})
	got, ok := parseTapeSetupCommand("export COPY=$TRACKED", state)
	require.True(t, ok)
	assert.Equal(t, map[string]string{"COPY": "from-step"}, got.Env)
}

func TestExportPathExpandsInsteadOfBecomingLiteral(t *testing.T) {
	t.Setenv("PATH", "/usr/bin"+string(os.PathListSeparator)+"/bin")
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
		Tape: "Type \"export PATH=$PATH:/opt/extra\" Enter\nType \"echo $PATH\" Enter",
	}
	require.NoError(t, expandCastTape(step))
	shell := step.Steps[len(step.Steps)-1]
	assert.Equal(t, "/usr/bin"+string(os.PathListSeparator)+"/bin:/opt/extra", shell.Env["PATH"])
}

func TestCompoundCdIsNotLiftedInStepsModeAndWarns(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
		Tape: "Type \"cd sub && pwd && echo X\" Enter\nType \"pwd\" Enter",
	}
	out := captureTapeLogOutput(t, func() { require.NoError(t, expandCastTape(step)) })

	require.Len(t, step.Steps, 4, "the compound line runs as a normal simulate+shell pair")
	assert.Equal(t, "cd sub && pwd && echo X", step.Steps[1].Command)
	assert.Empty(t, step.Steps[1].WorkingDirectory, "the compound line must not change the working directory of later commands")
	assert.Empty(t, step.Steps[3].WorkingDirectory)
	assert.Contains(t, out, "does not carry to later commands")
	assert.Contains(t, out, "cd sub && pwd && echo X")
}

func TestNonStateCompoundCommandDoesNotWarn(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
		Tape: "Type \"echo a && echo b\" Enter",
	}
	out := captureTapeLogOutput(t, func() { require.NoError(t, expandCastTape(step)) })
	assert.NotContains(t, out, "does not carry")
}

func TestTapeTextChangesShellState(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"cd sub && pwd", true},
		{"echo a; cd sub", true},
		{"(cd sub; ls)", true},
		{"true && export A=1", true},
		{"unset A && echo", true},
		{"echo cd export unset", false},
		{"ls -l | grep cd", false},
		{"cd 'unterminated", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tapeTextChangesShellState(tt.text), tt.text)
	}
}

func TestSessionHideAfterVisibleActivityReplaysLiterally(t *testing.T) {
	// Mirrors .context/ft-vhs/hideorder.tape: the first command runs before the
	// Hide block, so the block's export and cd must not be applied from the start.
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: `Type "echo FIRST=[$FOO] PWD=$PWD" Enter
Sleep 500ms
Hide
Type "export FOO=late" Enter
Type "cd sub" Enter
Show
Type "echo SECOND=[$FOO] PWD=$PWD" Enter`,
	}
	require.NoError(t, expandCastTape(step))

	assert.NotContains(t, step.Env, "FOO")
	assert.Empty(t, step.WorkingDirectory)
	want := []schema.WorkflowStep{
		{Type: "write", Text: "echo FIRST=[$FOO] PWD=$PWD"},
		{Type: "key", Key: "enter"},
		{Type: "pause", Duration: "500ms"},
		{Type: "hide"},
		{Type: "write", Text: "export FOO=late"},
		{Type: "key", Key: "enter"},
		{Type: "write", Text: "cd sub"},
		{Type: "key", Key: "enter"},
		{Type: "show"},
		{Type: "write", Text: "echo SECOND=[$FOO] PWD=$PWD"},
		{Type: "key", Key: "enter"},
	}
	assertWorkflowStepsEqual(t, want, step.Steps)
}

func TestSessionHideBeforeVisibleActivityStillLifts(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: "Hide\nType \"export FOO=early\" Enter\nShow\nType \"echo $FOO\" Enter",
	}
	require.NoError(t, expandCastTape(step))
	assert.Equal(t, "early", step.Env["FOO"])
	assertWorkflowStepsEqual(t, []schema.WorkflowStep{
		{Type: "write", Text: "echo $FOO"},
		{Type: "key", Key: "enter"},
	}, step.Steps)
}

func TestSessionSetupAfterLiteralHiddenReplayStaysInOrder(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: "Hide\nType \"echo prep\" Enter\nType \"export FOO=1\" Enter\nShow",
	}
	require.NoError(t, expandCastTape(step))
	assert.NotContains(t, step.Env, "FOO", "a setup command behind a replayed command must replay in order")
	assertWorkflowStepsEqual(t, []schema.WorkflowStep{
		{Type: "hide"},
		{Type: "write", Text: "echo prep"},
		{Type: "key", Key: "enter"},
		{Type: "write", Text: "export FOO=1"},
		{Type: "key", Key: "enter"},
		{Type: "show"},
	}, step.Steps)
}

func TestSessionScreenshotCountsAsVisibleActivity(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: "Screenshot a.png\nHide\nType \"export FOO=1\" Enter\nShow",
	}
	require.NoError(t, expandCastTape(step))
	assert.NotContains(t, step.Env, "FOO")
}

// speedTapeDirectives builds directives directly so these tests do not depend
// on the tape parser's @-modifier support.
func speedTapeDirectives() []asciicast.TapeDirective {
	return []asciicast.TapeDirective{
		{Kind: asciicast.TapeType, Value: "ls", Speed: "50ms"},
		{Kind: asciicast.TapeKey, Key: "enter"},
		{Kind: asciicast.TapeKey, Key: "backspace", Repeat: 5, Speed: "100ms"},
		{Kind: asciicast.TapeWait, Regex: "done", Timeout: "10s"},
		{Kind: asciicast.TapeWait, Regex: "later"},
	}
}

func TestSessionSpeedAndTimeoutModifiersMapToActions(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Timeout: "300s"}
	steps, err := translateTapeSession(step, speedTapeDirectives(), newTapeTrackedState(step))
	require.NoError(t, err)

	want := []schema.WorkflowStep{
		{Type: "write", Text: "ls", Rate: "50ms"},
		{Type: "key", Key: "enter"},
		{Type: "key", Key: "backspace", Repeat: 5, Interval: "100ms"},
		{Type: "wait", Regex: "done", Timeout: "10s"},
		{Type: "wait", Regex: "later", Timeout: "300s"},
	}
	require.Len(t, steps, len(want))
	for i := range want {
		assert.Equal(t, want[i].Type, steps[i].Type, "step %d", i)
		assert.Equal(t, want[i].Text, steps[i].Text, "step %d", i)
		assert.Equal(t, want[i].Key, steps[i].Key, "step %d", i)
		assert.Equal(t, want[i].Rate, steps[i].Rate, "step %d", i)
		assert.Equal(t, want[i].Interval, steps[i].Interval, "step %d", i)
		assert.Equal(t, want[i].Repeat, steps[i].Repeat, "step %d", i)
		assert.Equal(t, want[i].Regex, steps[i].Regex, "step %d", i)
		assert.Equal(t, want[i].Timeout, steps[i].Timeout, "step %d", i)
	}
}

func TestEnterWithRepeatIsNotFoldedIntoTypedCommand(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session"}
	directives := []asciicast.TapeDirective{
		{Kind: asciicast.TapeType, Value: "ls"},
		{Kind: asciicast.TapeKey, Key: "enter", Repeat: 3},
	}
	steps, err := translateTapeSession(step, directives, newTapeTrackedState(step))
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.Equal(t, "key", steps[1].Type)
	assert.Equal(t, 3, steps[1].Repeat, "the repeat count must not be dropped")
}

func TestStepsModeTypeSpeedBecomesSimulateRateAndWaitTimeoutIsDropped(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "steps"}
	directives := []asciicast.TapeDirective{
		{Kind: asciicast.TapeType, Value: "echo hi", Speed: "50ms"},
		{Kind: asciicast.TapeKey, Key: "enter"},
		{Kind: asciicast.TapeWait, Regex: "hi", Timeout: "10s"},
	}
	var children []schema.WorkflowStep
	out := captureTapeLogOutput(t, func() {
		var err error
		children, err = translateTapeSteps(step, directives, newTapeTrackedState(step))
		require.NoError(t, err)
	})
	require.Len(t, children, 2)
	assert.Equal(t, "50ms", children[0].Rate)
	assert.Contains(t, out, "no effect in mode: steps")
}

func TestUnsupportedTapeOutputExtensionNamesExtensionAndSupportedList(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "steps", Tape: "Output demo.webm\nType \"echo hi\" Enter"}
	err := expandCastTape(step)
	require.ErrorIs(t, err, ErrUnsupportedTapeOutputExtension)
	assert.Contains(t, err.Error(), "demo.webm")
	assert.Contains(t, err.Error(), ".webm")
	for _, ext := range tapeOutputExtensionList {
		assert.Contains(t, err.Error(), ext)
	}

	step = &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "steps", Tape: "Output demo\nType \"echo hi\" Enter"}
	err = expandCastTape(step)
	require.ErrorIs(t, err, ErrUnsupportedTapeOutputExtension)
	assert.Contains(t, err.Error(), "(none)")
}

func TestTapeOutputExtensionListMatchesSupportedMap(t *testing.T) {
	assert.Len(t, tapeOutputExtensionList, len(tapeOutputExtensions))
	for _, ext := range tapeOutputExtensionList {
		assert.Contains(t, tapeOutputExtensions, ext)
	}
}

func TestTapeOutputOverwritesOnlyWhenItSeedsAField(t *testing.T) {
	t.Run("seeded field overwrites", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "demo"}
		require.NoError(t, applyTapeOutputDirective(step, &asciicast.TapeDirective{Kind: asciicast.TapeOutput, Value: "x.gif"}))
		assert.True(t, step.CastOutput.TapeOutputFormats["gif"])
	})
	t.Run("explicit field wins and keeps no-clobber", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "demo", CastOutput: &schema.CastOutput{GIF: "explicit.gif"}}
		require.NoError(t, applyTapeOutputDirective(step, &asciicast.TapeDirective{Kind: asciicast.TapeOutput, Value: "x.gif"}))
		assert.Equal(t, "explicit.gif", step.CastOutput.GIF)
		assert.Empty(t, step.CastOutput.TapeOutputFormats)
	})
}

// runTapeCast validates and executes a tape-driven cast step, returning the recorded cast text.
func runTapeCast(t *testing.T, step *schema.WorkflowStep) (string, error) {
	t.Helper()
	require.NoError(t, iolib.Initialize())
	require.NoError(t, (&CastHandler{}).Validate(step))
	_, err := (&CastHandler{}).Execute(context.Background(), step, NewVariables())
	if err != nil {
		return "", err
	}
	raw, readErr := os.ReadFile(step.CastOutput.Cast)
	require.NoError(t, readErr)
	return string(raw), nil
}

func TestTapeStepsRecordingHasLiteralTextAndNoStepChrome(t *testing.T) {
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps", Width: 80, Height: 10,
		CastOutput: &schema.CastOutput{Cast: castPath},
		Tape:       `Type "echo '{{.Names}}'" Enter`,
	}
	recording, err := runTapeCast(t, step)
	require.NoError(t, err)

	assert.Contains(t, recording, "{{.Names}}")
	assert.NotContains(t, recording, "<no value>")
	assert.NotContains(t, recording, "_step_", "synthetic child names must not appear in the recording")
	assert.NotContains(t, recording, "completed", "the step footer must not appear in the recording")
}

func TestTapeStepsChromeFollowsExplicitShowLabels(t *testing.T) {
	withLabels := &schema.WorkflowStep{Name: "demo", Show: &schema.ShowConfig{Labels: BoolPtr(true)}}
	assert.Nil(t, tapeChildShow(withLabels), "an explicit show.labels on the cast step is inherited by the children")

	other := &schema.ShowConfig{Command: BoolPtr(true)}
	step := &schema.WorkflowStep{Name: "demo", Show: other}
	show := tapeChildShow(step)
	require.NotNil(t, show)
	require.NotNil(t, show.Labels)
	assert.False(t, *show.Labels)
	assert.Same(t, other.Command, show.Command, "other show settings carry over")
	assert.Nil(t, other.Labels, "the cast step's own show config is not mutated")
}

func TestTapeOutputOverwritesOnRerunButHandWrittenOutputDoesNot(t *testing.T) {
	dir := t.TempDir()
	newStep := func(tape string, output *schema.CastOutput) *schema.WorkflowStep {
		return &schema.WorkflowStep{
			Name: "demo", Type: schema.TaskTypeCast, Mode: "steps", Width: 80, Height: 10,
			CastOutput: output, Tape: tape,
		}
	}

	t.Run("tape output", func(t *testing.T) {
		ascii := filepath.Join(dir, "tape.ascii")
		tape := "Output " + ascii + "\nType \"echo hi\" Enter"
		castPath := filepath.Join(dir, "tape.cast")
		for run := 0; run < 2; run++ {
			_, err := runTapeCast(t, newStep(tape, &schema.CastOutput{Cast: castPath}))
			require.NoError(t, err, "run %d", run)
		}
		assert.FileExists(t, ascii)
	})

	t.Run("hand-written output", func(t *testing.T) {
		ascii := filepath.Join(dir, "hand.ascii")
		castPath := filepath.Join(dir, "hand.cast")
		tape := "Type \"echo hi\" Enter"
		_, err := runTapeCast(t, newStep(tape, &schema.CastOutput{Cast: castPath, ASCII: ascii}))
		require.NoError(t, err)
		_, err = runTapeCast(t, newStep(tape, &schema.CastOutput{Cast: castPath, ASCII: ascii}))
		require.ErrorIs(t, err, errUtils.ErrRenderOutputExists)
	})
}

func TestCastSessionShellInitPrecedence(t *testing.T) {
	const key = "cast.session.shell_init"
	t.Cleanup(func() { viper.Set(key, "") })

	viper.Set(key, "")
	assert.Equal(t, "", castSessionShellInit(&schema.WorkflowStep{}), "empty means the default (clean)")

	viper.Set(key, "user")
	assert.Equal(t, "user", castSessionShellInit(&schema.WorkflowStep{}), "atmos.yaml default applies")
	assert.Equal(t, "clean", castSessionShellInit(&schema.WorkflowStep{ShellInit: "clean"}), "the step wins over atmos.yaml")
	assert.Equal(t, "/tmp/rc", castSessionShellInit(&schema.WorkflowStep{ShellInit: " /tmp/rc "}))
	assert.Equal(t, "user", castSessionShellInit(&schema.WorkflowStep{ShellInit: "  "}), "blank is unset")
}

func TestSessionBlockInheritsParentShellInit(t *testing.T) {
	parent := &schema.WorkflowStep{ShellInit: "user"}
	inherits := &schema.WorkflowStep{}
	inheritCastSessionBlockDefaults(parent, inherits)
	assert.Equal(t, "user", inherits.ShellInit)

	own := &schema.WorkflowStep{ShellInit: "clean"}
	inheritCastSessionBlockDefaults(parent, own)
	assert.Equal(t, "clean", own.ShellInit)
}

func TestShellInitSurvivesTaskStepConversion(t *testing.T) {
	task := schema.Task{Name: "demo", Type: schema.TaskTypeCast, ShellInit: "user"}
	step := task.ToWorkflowStep()
	assert.Equal(t, "user", step.ShellInit)
	back := schema.TaskFromWorkflowStep(&step)
	assert.Equal(t, "user", back.ShellInit)
}

func TestCastStepFieldsListShellInit(t *testing.T) {
	assert.Contains(t, castStepFields, "shell_init")
	assert.Contains(t, castStepFields, "source")
}
