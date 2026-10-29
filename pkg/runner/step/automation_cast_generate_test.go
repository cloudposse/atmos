package step

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/castplan"
	"github.com/cloudposse/atmos/pkg/schema"
)

// declaredGeneration drives AutomationLibrary.GenerateCast directly with a declaration plan.
type declaredGeneration struct {
	dir  string
	plan *castplan.Plan
}

// castTestDefaults makes typed text instantaneous so tests do not wait on typing pacing.
func castTestDefaults() map[string]any {
	return map[string]any{"simulate": map[string]any{"rate": "0s", "duration": "0s", "interval": "0s"}}
}

func newDeclaredGeneration(t *testing.T, actions ...castplan.Action) *declaredGeneration {
	t.Helper()
	initShellTestIO(t)
	return &declaredGeneration{
		dir: t.TempDir(),
		plan: &castplan.Plan{
			Configuration: map[string]any{
				"output":   map[string]any{"cast": "demo.cast", "mode": "raw"},
				"defaults": castTestDefaults(),
			},
			Actions: actions,
		},
	}
}

func (g *declaredGeneration) call() *api.StepCall {
	return &api.StepCall{Type: "cast", WorkingDirectory: g.dir}
}

func (g *declaredGeneration) castPath() string { return filepath.Join(g.dir, "demo.cast") }

// validator checks the staged recording exactly as the Starlark host does, minus the user policy.
func (g *declaredGeneration) validator() func(string) error {
	return func(path string) error {
		_, err := asciicast.ValidateDeclaration(path, g.plan)
		return err
	}
}

func (g *declaredGeneration) generate(ctx context.Context) (string, error) {
	return NewAutomationLibrary(nil, nil).GenerateCast(ctx, g.plan, g.call(), g.validator())
}

// recordedText returns the concatenated output of the committed recording.
func (g *declaredGeneration) recordedText(t *testing.T) string {
	t.Helper()
	_, events, err := asciicast.ReadEvents(g.castPath())
	require.NoError(t, err)
	var text strings.Builder
	for _, event := range events {
		if event.Stream == "o" || event.Stream == "e" {
			text.WriteString(event.Data)
		}
	}
	return text.String()
}

// fakeAtmosRun declares a recorded command that runs the test binary as a fake atmos in the given mode.
func fakeAtmosRun(t *testing.T, mode string, extraEnv map[string]string) castplan.Action {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	env := map[string]any{"_ATMOS_STEP_FAKE": mode}
	for key, value := range extraEnv {
		env[key] = value
	}
	return castplan.Action{
		Kind:   "run",
		Fields: map[string]any{"command": strconv.Quote(exe), "env": env, "interval": "0s"},
	}
}

func simulateAction(text string) castplan.Action {
	return castplan.Action{Kind: "simulate", Fields: map[string]any{"text": text}}
}

func TestGenerateCastRejectsInvalidDeclarationsWithoutRecording(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*castplan.Plan)
		want     error
		contains string
	}{
		{
			name:     "no output path",
			mutate:   func(p *castplan.Plan) { p.Configuration = nil },
			want:     errUtils.ErrAutomation,
			contains: "requires output.cast",
		},
		{
			name:     "unresolvable working directory template",
			mutate:   func(p *castplan.Plan) { p.Configuration["working_directory"] = "{{ .nope" },
			contains: "failed to parse template",
		},
		{
			name:     "invalid step timeout",
			mutate:   func(p *castplan.Plan) { p.Configuration["timeout"] = "soon" },
			contains: "invalid step timeout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newDeclaredGeneration(t, simulateAction("never recorded"))
			tt.mutate(g.plan)
			_, err := g.generate(t.Context())
			require.Error(t, err)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			}
			require.ErrorContains(t, err, tt.contains)
			require.NoFileExists(t, g.castPath())
		})
	}
}

func TestGenerateCastRequiresDeclarationAndInvocation(t *testing.T) {
	initShellTestIO(t)
	library := NewAutomationLibrary(nil, nil)
	validate := func(string) error { return nil }
	_, err := library.GenerateCast(t.Context(), nil, &api.StepCall{}, validate)
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	_, err = library.GenerateCast(t.Context(), &castplan.Plan{}, nil, validate)
	require.ErrorIs(t, err, errUtils.ErrAutomation)
}

func TestGenerateCastDefaultsOutputModeToRawAndRecordsEvidence(t *testing.T) {
	g := newDeclaredGeneration(t, simulateAction("# visible narration"))
	g.plan.Configuration["output"] = map[string]any{"cast": "demo.cast"}
	path, err := g.generate(t.Context())
	require.NoError(t, err)
	require.Equal(t, g.castPath(), path)
	require.Contains(t, g.recordedText(t), "# visible narration")
	// The committed recording is the evidence ValidateDeclaration accepts.
	_, err = asciicast.ValidateDeclaration(path, g.plan)
	require.NoError(t, err)
}

func TestGenerateCastRequiresValidatorAndKeepsPreviousRecording(t *testing.T) {
	g := newDeclaredGeneration(t, simulateAction("new content"))
	require.NoError(t, os.WriteFile(g.castPath(), []byte("previous recording"), 0o600))
	_, err := NewAutomationLibrary(nil, nil).GenerateCast(t.Context(), g.plan, g.call(), nil)
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	require.ErrorContains(t, err, "validator is required")
	content, readErr := os.ReadFile(g.castPath())
	require.NoError(t, readErr)
	require.Equal(t, "previous recording", string(content))
}

func TestPublishDoesNotCommitForCancelledRun(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	run := &declaredCast{ctx: ctx}
	validated := false
	path, err := run.publish(func(string) error {
		validated = true
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, path)
	require.False(t, validated, "a cancelled run must not reach the recording validator")
}

func TestGenerateCastFailsWhenDeclarationCannotBeFingerprinted(t *testing.T) {
	g := newDeclaredGeneration(t, castplan.Action{Kind: "simulate", Fields: map[string]any{"text": math.NaN()}})
	require.NoError(t, os.WriteFile(g.castPath(), []byte("previous recording"), 0o600))
	_, err := g.generate(t.Context())
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	require.ErrorContains(t, err, "cast declaration")
	content, readErr := os.ReadFile(g.castPath())
	require.NoError(t, readErr)
	require.Equal(t, "previous recording", string(content))
}

func TestGenerateCastReportsRenderFailureAfterRecordingIsCommitted(t *testing.T) {
	g := newDeclaredGeneration(t, simulateAction("rendered"))
	blocker := filepath.Join(g.dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("a file, not a directory"), 0o600))
	g.plan.Configuration["output"] = map[string]any{"cast": "demo.cast", "mode": "raw", "ascii": "blocker/out.txt"}
	_, err := g.generate(t.Context())
	require.Error(t, err)
	require.ErrorContains(t, err, "blocker")
	// Rendering runs after the validated recording is committed, so the cast itself stays valid evidence.
	_, validateErr := asciicast.ValidateDeclaration(g.castPath(), g.plan)
	require.NoError(t, validateErr)
}

func TestGenerateCastReportsMarkerScreenshotFailure(t *testing.T) {
	g := newDeclaredGeneration(
		t,
		simulateAction("frame"),
		castplan.Action{Kind: "screenshot", Fields: map[string]any{"path": "blocker/frame.png"}},
	)
	require.NoError(t, os.WriteFile(filepath.Join(g.dir, "blocker"), []byte("a file"), 0o600))
	_, err := g.generate(t.Context())
	require.ErrorContains(t, err, "failed to render marker screenshot")
	require.ErrorContains(t, err, "blocker")
}

func TestDeclaredChildResolvesRelativeWorkingDirectory(t *testing.T) {
	base := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "elsewhere")
	run := &declaredCast{
		step: &schema.WorkflowStep{},
		call: &api.StepCall{WorkingDirectory: base},
	}
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{name: "relative joins the invocation directory", dir: "fixtures", want: filepath.Join(base, "fixtures")},
		{name: "absolute is kept", dir: absolute, want: absolute},
		{name: "unset stays unset", dir: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]any{"text": "x"}
			if tt.dir != "" {
				fields["working_directory"] = tt.dir
			}
			child, err := run.child(&castplan.Action{Kind: "simulate", Fields: fields})
			require.NoError(t, err)
			require.Equal(t, tt.want, child.WorkingDirectory)
		})
	}
}
