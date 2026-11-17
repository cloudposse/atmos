package step

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var (
	ErrCastStepRequiresSteps         = errUtils.ErrCastStepRequiresSteps
	ErrCastSessionRequiresActions    = errUtils.ErrCastSessionRequiresActions
	ErrInvalidCastMode               = errUtils.ErrInvalidCastMode
	ErrWriteActionRequiresText       = errUtils.ErrWriteActionRequiresText
	ErrKeyActionRequiresKey          = errUtils.ErrKeyActionRequiresKey
	ErrPauseActionRequiresDuration   = errUtils.ErrPauseActionRequiresDuration
	ErrWaitActionRequiresTextOrRegex = errUtils.ErrWaitActionRequiresTextOrRegex
	ErrUnsupportedSessionAction      = errUtils.ErrUnsupportedSessionAction
	ErrInvalidSimulateMode           = errUtils.ErrInvalidSimulateMode
	ErrSimulateTypedRequiresText     = errUtils.ErrSimulateTypedRequiresText
	ErrInvalidSimulateJitter         = errUtils.ErrInvalidSimulateJitter
	ErrUnsupportedPromptStyle        = errUtils.ErrUnsupportedPromptStyle
	ErrScreenshotActionRequiresPath  = errUtils.ErrScreenshotActionRequiresPath
)

const wrappedQuotedErrorFormat = "%w: %q"

// castSessionStepType names a steps-mode child that opens a real,
// interactive PTY session for its own nested `steps:` (write/key/pause/wait/
// simulate actions), then returns control to the enclosing steps-mode cast.
// This is the seam that lets one recording open on real interactive prompts
// and fall through to ordinary, non-interactive command execution -- no
// separate top-level `mode: session` cast step required.
const castSessionStepType = "session"

const (
	defaultCastTypeDelay      = 35 * time.Millisecond
	defaultCastPromptDelay    = 350 * time.Millisecond
	defaultCastEnterDelay     = 550 * time.Millisecond
	defaultCastStepPauseDelay = 700 * time.Millisecond
	castCursorShow            = "\x1b[?25h"

	castWhitespaceBoundaryMinFactor     = 1.6
	castWhitespaceBoundaryJitterFactor  = 0.5
	castPunctuationBoundaryMinFactor    = 1.3
	castPunctuationBoundaryJitterFactor = 0.4
	castCommentTypingFactor             = 1.1
	castJitterHashOffset                = uint64(14695981039346656037)
	castJitterHashPrime                 = uint64(1099511628211)
	castJitterScale                     = uint64(1000)
	castJitterScaleMax                  = castJitterScale - 1
)

type CastHandler struct {
	BaseHandler
}

type castTypedLineOptions struct {
	Prompt     *schema.SimulatePrompt
	Line       string
	WriteRate  time.Duration
	Jitter     float64
	EnterDelay time.Duration
	Cursor     bool
	SkipPrompt bool
}

func init() {
	Register(&CastHandler{
		BaseHandler: NewBaseHandler(schema.TaskTypeCast, CategoryCommand, false),
	})
}

func (h *CastHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.CastHandler.Validate")()

	applyCastRecordingDefaults(step)
	if err := expandCastTape(step); err != nil {
		return err
	}
	mode := castMode(step)
	switch mode {
	case "steps":
		return validateCastStepsMode(step)
	case "session":
		return validateCastSessionMode(step)
	default:
		return fmt.Errorf("%w: %q mode %q", ErrInvalidCastMode, step.Name, step.Mode)
	}
}

func (h *CastHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.CastHandler.Execute")()

	return h.ExecuteWithWorkflow(ctx, step, vars, nil)
}

func (h *CastHandler) ExecuteWithWorkflow(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	defer perf.Track(nil, "step.CastHandler.ExecuteWithWorkflow")()

	applyCastRecordingDefaults(step)
	rec, restore, err := startStepRecorder(step, vars)
	if err != nil {
		return nil, err
	}
	runErr := runCastBody(ctx, rec, step, vars, workflow)
	restore()
	runErr = finishCastRecording(rec, step, runErr)
	result := NewStepResult(rec.Path()).WithMetadata("cast", rec.Path())
	if step.CastOutput != nil {
		result.WithMetadata("gif", step.CastOutput.GIF)
		result.WithMetadata("mp4", step.CastOutput.MP4)
	}
	return result, runErr
}

func runCastBody(ctx context.Context, rec *asciicast.Recorder, castStep *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) error {
	if err := runCastTapeRequirements(ctx, castStep, vars); err != nil {
		return err
	}
	switch castMode(castStep) {
	case "steps":
		return runCastStepMode(ctx, rec, castStep, vars, workflow)
	case "session":
		return runCastSessionMode(ctx, rec, castStep, vars, workflow)
	default:
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrInvalidCastMode, castStep.Mode)
	}
}

// runCastTapeRequirements checks any tool names collected from a tape's
// Require directives (via castStep.Tools) by synthesizing and running a real
// {type: require} step through the existing step registry -- genuine
// black-box reuse: identical error/hint UX to a hand-written type: require
// step, and it works uniformly for both cast modes since a PATH check has
// nothing PTY-specific about it. A no-op when the step has no tape-derived
// (or hand-authored) Tools.
func runCastTapeRequirements(ctx context.Context, castStep *schema.WorkflowStep, vars *Variables) error {
	if len(castStep.Tools) == 0 {
		return nil
	}
	// Preflight sees the same environment as the recording without persisting
	// session-local overrides or requirement results in the enclosing workflow.
	requireVars := vars.Clone()
	if err := applyCastStepEnv(castStep, requireVars); err != nil {
		return err
	}
	requireStep := &schema.WorkflowStep{
		Name:  castStep.Name + "_require",
		Type:  "require",
		Tools: castStep.Tools,
	}
	_, err := NewStepExecutorWithVars(requireVars).Execute(ctx, requireStep)
	return err
}

func runCastStepMode(ctx context.Context, rec *asciicast.Recorder, castStep *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) error {
	normalizeCastOutputMode(castStep)
	if err := applyCastStepEnv(castStep, vars); err != nil {
		return err
	}
	runErr := runCastSteps(ctx, rec, castStep, vars, workflow)
	if err := recordCastPromptWithCursor(nil, castStepHasVisibleCursor(castStep)); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

func runCastSteps(ctx context.Context, rec *asciicast.Recorder, castStep *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) error {
	executor := NewStepExecutorWithVars(vars)
	if workflow != nil {
		executor.SetWorkflow(workflow)
	}
	prevWasSession := false
	runner := castChildStepRunner{ctx: ctx, recorder: rec, castStep: castStep, vars: vars, executor: executor, workflow: workflow, prevWasSession: &prevWasSession}
	return runner.runSteps()
}

func (r castChildStepRunner) runSteps() error {
	conditionContext := schema.ConditionContext{Status: schema.ConditionPredicateSuccess}
	var runErr error
	// fail accumulates a child error and flips the condition status so that
	// subsequent `when:` clauses observe the failure.
	fail := func(err error) {
		runErr = errors.Join(runErr, err)
		conditionContext.Status = schema.ConditionPredicateFailure
	}
	// prevWasSession tracks whether the previous child was a `type: session`
	// block: that block's real shell always leaves its own next prompt
	// visible the moment the block exits (the shell is idle, waiting for
	// input, right up until the recording moves on) -- a following
	// `type: simulate` narration line must not draw a second prompt on top
	// of it.
	for i := range r.castStep.Steps {
		child := &r.castStep.Steps[i]
		if !child.When.EvaluateWithImplicitSuccess(conditionContext) {
			continue
		}
		prepareCastChildStep(r.castStep, child, i)
		skipPrompt := *r.prevWasSession && child.Type == schema.TaskTypeSimulate
		if err := r.run(child, skipPrompt); err != nil {
			fail(err)
		}
		if child.Type != castPlaybackStepType {
			*r.prevWasSession = child.Type == castSessionStepType
		}
	}
	return runErr
}

// runCastChildStep executes one child step of a steps-mode cast: simulate
// steps replay scripted output, screenshot steps record a marker (no PTY is
// needed, only the shared recorder both modes install identically, so
// Screenshot works the same in mode: steps as in mode: session), everything
// else runs through the executor followed by the configured input pause. The
// skipPrompt flag suppresses a simulate step's own prompt draw when a real
// prompt is already visible (see the prevWasSession tracking in
// runCastStepMode, and castSessionActions' Fn wrapper, which always skips it
// since a live session's real shell prompt is always already showing before
// any of its actions run).
type castChildStepRunner struct {
	ctx            context.Context
	recorder       *asciicast.Recorder
	prevWasSession *bool
	castStep       *schema.WorkflowStep
	vars           *Variables
	executor       *StepExecutor
	workflow       *schema.WorkflowDefinition
}

func (r castChildStepRunner) run(child *schema.WorkflowStep, skipPrompt bool) error {
	switch child.Type {
	case castPlaybackStepType:
		return r.runPlaybackGroup(child)
	case schema.TaskTypeSimulate:
		return runCastSimulateStep(r.ctx, r.castStep, child, r.vars, skipPrompt)
	case schema.TaskTypeScreenshot:
		iolib.RecordMarker(child.Path)
		return nil
	case castSessionStepType:
		return r.runSessionBlock(child)
	default:
		if _, err := r.executor.Execute(r.ctx, child); err != nil {
			return err
		}
		delay, err := castStepPauseDelay(child)
		if err != nil {
			return err
		}
		return sleepCastInput(r.ctx, delay)
	}
}

// runSessionBlock opens a real PTY session scoped to one steps-mode
// child: session-level config (Shell/WorkingDirectory/Width/Height/
// WriteRate/KeyInterval/Defaults) falls back to the enclosing cast step's
// when unset, exactly like a top-level `mode: session` cast step's config
// -- the block's own `steps:` are its write/key/pause/wait/simulate/exec
// actions, converted via the same castSessionActions used by top-level
// session mode. WorkingDirectory is inherited earlier, by
// prepareCastChildStep, before this runs.
func (r castChildStepRunner) runSessionBlock(block *schema.WorkflowStep) error {
	ctx, rec, castStep, vars, workflow := r.ctx, r.recorder, r.castStep, r.vars, r.workflow
	inheritCastSessionBlockDefaults(castStep, block)
	writeRate, err := parseDurationDefault(block.WriteRate, 40*time.Millisecond)
	if err != nil {
		return err
	}
	keyInterval, err := parseDurationDefault(block.KeyInterval, 20*time.Millisecond)
	if err != nil {
		return err
	}
	env, err := castRecorderEnv(block, vars)
	if err != nil {
		return err
	}
	if err := applySessionPromptEnv(block, env); err != nil {
		return err
	}
	return asciicast.RunSession(ctx, &asciicast.SessionOptions{
		Recorder:    rec,
		Shell:       block.Shell,
		ShellInit:   castSessionShellInit(block),
		Dir:         block.WorkingDirectory,
		Env:         env,
		Width:       block.Width,
		Height:      block.Height,
		WriteRate:   writeRate,
		KeyInterval: keyInterval,
		Actions:     castSessionActions(ctx, block, vars, workflow),
	})
}

func inheritCastSessionBlockDefaults(parent, block *schema.WorkflowStep) {
	if block.Shell == "" {
		block.Shell = parent.Shell
	}
	if block.ShellInit == "" {
		block.ShellInit = parent.ShellInit
	}
	if block.Width <= 0 {
		block.Width = parent.Width
	}
	if block.Height <= 0 {
		block.Height = parent.Height
	}
	if block.WriteRate == "" {
		block.WriteRate = parent.WriteRate
	}
	if block.KeyInterval == "" {
		block.KeyInterval = parent.KeyInterval
	}
	if block.Defaults == nil {
		block.Defaults = parent.Defaults
	}
}

// castSessionShellInit resolves how a session shell initializes: the step's
// own shell_init (a session block inherits its parent's), then the atmos.yaml
// cast.session.shell_init default. An empty result means the default, "clean".
// Any non-empty value is accepted here: "clean" and "user" are keywords and
// anything else is an rc file path whose existence the session checks when it
// starts.
func castSessionShellInit(step *schema.WorkflowStep) string {
	return firstNonEmpty(strings.TrimSpace(step.ShellInit), strings.TrimSpace(viper.GetString("cast.session.shell_init")))
}

func applyCastStepEnv(castStep *schema.WorkflowStep, vars *Variables) error {
	if len(castStep.Env) == 0 {
		return nil
	}
	resolvedEnv, err := vars.ResolveStepEnvMap(castStep, castStep.Env)
	if err != nil {
		return fmt.Errorf("step '%s': %w", castStep.Name, err)
	}
	for key, value := range resolvedEnv {
		vars.SetEnv(key, value)
	}
	return nil
}

func normalizeCastOutputMode(castStep *schema.WorkflowStep) {
	if castStep.Output == "" && castStep.CastOutput != nil {
		castStep.Output = castStep.CastOutput.Mode
	}
}

func prepareCastChildStep(castStep, child *schema.WorkflowStep, index int) {
	if child.Name == "" {
		child.Name = fmt.Sprintf("%s_step_%d", castStep.Name, index+1)
	}
	if child.WorkingDirectory == "" {
		child.WorkingDirectory = castStep.WorkingDirectory
	}
	if child.Type == "" {
		child.Type = schema.TaskTypeShell
	}
	if child.Output == "" {
		child.Output = castStep.Output
	}
	if child.Show == nil {
		child.Show = castStep.Show
	}
	if child.Type == schema.TaskTypeSimulate {
		applyCastSimulateDefaults(castStep, child)
	}
}

func runCastSessionMode(ctx context.Context, rec *asciicast.Recorder, castStep *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) error {
	writeRate, err := parseDurationDefault(castStep.WriteRate, 40*time.Millisecond)
	if err != nil {
		return err
	}
	keyInterval, err := parseDurationDefault(castStep.KeyInterval, 20*time.Millisecond)
	if err != nil {
		return err
	}
	env, err := castRecorderEnv(castStep, vars)
	if err != nil {
		return err
	}
	if err := applySessionPromptEnv(castStep, env); err != nil {
		return err
	}
	return asciicast.RunSession(ctx, &asciicast.SessionOptions{
		Recorder:    rec,
		Shell:       castStep.Shell,
		ShellInit:   castSessionShellInit(castStep),
		Dir:         castStep.WorkingDirectory,
		Env:         env,
		Width:       castStep.Width,
		Height:      castStep.Height,
		WriteRate:   writeRate,
		KeyInterval: keyInterval,
		Actions:     castSessionActions(ctx, castStep, vars, workflow),
	})
}

// applySessionPromptEnv sets PS1 to the same styled ("command") prompt text
// mode: steps' simulate rendering uses, so a real shell's own prompt output
// (printed before/after every real command a session action types) matches
// the styling of any type: simulate narration mixed into the same session --
// without this, only the narration lines would be themed and the real
// prompt would stay plain, unstyled shell output. A caller-supplied PS1 is
// left untouched.
func applySessionPromptEnv(castStep *schema.WorkflowStep, env map[string]string) error {
	if _, ok := env["PS1"]; ok {
		return nil
	}
	prompt, err := renderCastPrompt(sessionPromptDefault(castStep))
	if err != nil {
		return err
	}
	env["PS1"] = prompt
	return nil
}

// sessionPromptDefault resolves the same prompt a type: simulate child of
// this cast step would use: the step's own SimulatePrompt if set, else
// castStep.Defaults.Simulate.Prompt, else nil (renderCastPrompt's built-in
// "> "/"command" fallback).
func sessionPromptDefault(castStep *schema.WorkflowStep) *schema.SimulatePrompt {
	if castStep.SimulatePrompt != nil {
		return castStep.SimulatePrompt
	}
	if castStep.Defaults != nil && castStep.Defaults.Simulate != nil {
		return castStep.Defaults.Simulate.Prompt
	}
	return nil
}

// castSessionActions converts a session-mode cast step's children into
// SessionActions. "write"/"key"/"pause"/"wait" drive the real interactive
// PTY as before. Every other type -- "simulate" and any registered step type
// (e.g. "shell", "atmos", "script") -- runs through the same StepExecutor
// mode: steps uses, via a callback: this is what lets a recording start
// interactive (real prompts driving the PTY) and then fall through to
// non-interactive, real command execution (e.g. a real `terraform apply`)
// without ever needing the PTY to answer that command's terminal
// capability-probe escape sequences, since the command never runs inside the
// PTY at all -- it runs the same way a mode: steps `type: shell` step does.
func castSessionActions(ctx context.Context, castStep *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) []asciicast.SessionAction {
	steps := castStep.Steps
	executor := NewStepExecutorWithVars(vars)
	if workflow != nil {
		executor.SetWorkflow(workflow)
	}
	runner := castChildStepRunner{ctx: ctx, castStep: castStep, vars: vars, executor: executor, workflow: workflow}
	actions := make([]asciicast.SessionAction, 0, len(steps))
	for i := range steps {
		child := &steps[i]
		if child.Type == castPlaybackStepType {
			group := *castStep
			group.Steps = child.Steps
			actions = append(actions, asciicast.SessionAction{
				Type: castPlaybackStepType, Duration: child.Duration,
				Actions: castSessionActions(ctx, &group, vars, workflow),
			})
			continue
		}
		if isCastSessionPTYAction(child.Type) {
			actions = append(actions, asciicast.SessionAction{
				Type:     child.Type,
				Text:     child.Text,
				Regex:    child.Regex,
				Key:      child.Key,
				Duration: child.Duration,
				Timeout:  child.Timeout,
				Rate:     child.Rate,
				Interval: child.Interval,
				Repeat:   child.Repeat,
			})
			continue
		}
		prepareCastChildStep(castStep, child, i)
		actions = append(actions, asciicast.SessionAction{
			// RunSession dispatches callbacks only for simulate actions. The
			// original child type remains available to runCastChildStep through
			// this closure, while the session dispatcher invokes it correctly.
			Type: schema.TaskTypeSimulate,
			// A live session's real shell prompt is always already visible
			// before any scripted action runs (even the very first one --
			// the freshly spawned shell shows its own prompt immediately),
			// so a simulate action mixed into a session must never draw its
			// own prompt on top of it.
			Fn: func() error { return runner.run(child, true) },
		})
	}
	return actions
}

// isCastSessionPTYAction reports whether a session action type drives the
// real interactive PTY directly, as opposed to running through the step
// executor (simulate, and any registered step type). The hide and show types
// toggle recording of the PTY output, so they belong to the PTY session too.
func isCastSessionPTYAction(actionType string) bool {
	switch actionType {
	case "write", "key", "pause", "wait", "hide", "show":
		return true
	default:
		return false
	}
}

func castCommandArgs(step *schema.WorkflowStep) []string {
	command := strings.TrimSpace(step.Command)
	if command != "" {
		return strings.Fields(command)
	}
	return nil
}

func renderCastOutputs(step *schema.WorkflowStep, castPath string) error {
	if step.CastOutput == nil {
		return nil
	}
	return asciicast.Render(castPath, &asciicast.RenderOptions{
		GIF:   step.CastOutput.GIF,
		MP4:   step.CastOutput.MP4,
		HTML:  step.CastOutput.HTML,
		ASCII: step.CastOutput.ASCII,
		PNG:   step.CastOutput.PNG,
		JPEG:  step.CastOutput.JPG,
		// Tape-derived Output targets replace an existing file, like VHS.
		OverwriteFormats: step.CastOutput.TapeOutputFormats,
	})
}

func castMode(step *schema.WorkflowStep) string {
	mode := strings.TrimSpace(step.Mode)
	if mode == "" {
		return "steps"
	}
	return mode
}

func finishCastRecording(rec *asciicast.Recorder, step *schema.WorkflowStep, runErr error) error {
	if runErr != nil {
		// Discard failed output without replacing an existing recording.
		discardErr := rec.Discard()
		ui.Warningf("Cast discarded (recording failed): %s", rec.Path())
		if discardErr != nil {
			return errors.Join(runErr, discardErr)
		}
		return runErr
	}
	if err := rec.Close(); err != nil {
		return err
	}
	if step.CastOutput == nil || !step.CastOutput.Intermediate {
		ui.Successf("Cast recorded: %s", rec.Path())
	}
	if err := renderCastOutputs(step, rec.Path()); err != nil {
		return err
	}
	return asciicast.RenderMarkerScreenshots(rec.Path())
}
