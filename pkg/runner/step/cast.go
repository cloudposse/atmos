package step

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
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

func validateCastStepsMode(step *schema.WorkflowStep) error {
	if len(step.Steps) == 0 {
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrCastStepRequiresSteps, step.Name)
	}
	if step.Jitter < 0 || step.Jitter > 1 {
		return fmt.Errorf("%w: %v", ErrInvalidSimulateJitter, step.Jitter)
	}
	for i := range step.Steps {
		child := &step.Steps[i]
		switch child.Type {
		case schema.TaskTypeSimulate:
			clone := *child
			applyCastSimulateDefaults(step, &clone)
			if err := validateCastSimulateStep(&clone); err != nil {
				return fmt.Errorf("cast simulate step %d: %w", i+1, err)
			}
		case castSessionStepType:
			if err := validateCastSessionMode(child); err != nil {
				return fmt.Errorf("cast session step %d: %w", i+1, err)
			}
		}
	}
	return nil
}

func validateCastSessionMode(step *schema.WorkflowStep) error {
	if len(step.Steps) == 0 {
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrCastSessionRequiresActions, step.Name)
	}
	for i := range step.Steps {
		if err := validateCastSessionAction(step, &step.Steps[i]); err != nil {
			return fmt.Errorf("cast session action %d: %w", i+1, err)
		}
	}
	return nil
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
	runErr := runCastBody(ctx, step, vars, workflow)
	restore()
	if runErr != nil {
		// A failed recording is discarded so it never replaces a previously
		// committed cast at the output path.
		if discardErr := rec.Discard(); discardErr != nil {
			runErr = errors.Join(runErr, discardErr)
		}
		ui.Warningf("Cast discarded (recording failed): %s", rec.Path())
	} else if closeErr := rec.Close(); closeErr != nil {
		runErr = closeErr
	} else {
		ui.Successf("Cast recorded: %s", rec.Path())
		runErr = renderCastOutputs(step, rec.Path())
	}
	result := NewStepResult(rec.Path()).WithMetadata("cast", rec.Path())
	if step.CastOutput != nil {
		result.WithMetadata("gif", step.CastOutput.GIF)
		result.WithMetadata("mp4", step.CastOutput.MP4)
	}
	return result, runErr
}

func startStepRecorder(step *schema.WorkflowStep, vars *Variables) (*asciicast.Recorder, func(), error) {
	applyCastRecordingDefaults(step)
	if vars == nil {
		vars = NewVariables()
	}
	path := ""
	if step.CastOutput != nil {
		path = step.CastOutput.Cast
	}
	title, err := vars.Resolve(step.Title)
	if err != nil {
		return nil, nil, err
	}
	env, err := castRecorderEnv(step, vars)
	if err != nil {
		return nil, nil, err
	}
	uiRestore := forceRecordingUIEnv(env)
	width, height := castRecorderDimensions(step)
	outputRate, err := parseDurationDefault(step.Rate, 0)
	if err != nil {
		uiRestore()
		return nil, nil, err
	}
	rec, err := asciicast.Start(&asciicast.Options{
		Path:       path,
		BasePath:   viper.GetString("cast.recording.base_path"),
		Name:       step.Name,
		Title:      title,
		Command:    castCommandArgs(step),
		Width:      width,
		Height:     height,
		RecordIn:   viper.GetBool("cast.recording.input"),
		Explicit:   path != "",
		Overwrite:  true, // Declarative steps own their output path; commit replaces it.
		Env:        env,
		OutputRate: outputRate,
	})
	if err != nil {
		uiRestore()
		return nil, nil, err
	}
	ioRestore := iolib.SetRecorder(rec)
	return rec, func() {
		ioRestore()
		uiRestore()
	}, nil
}

// castUIEnvMu and castUIEnvDepth guard forceRecordingUIEnv against nested or
// parallel-sibling `type: cast` steps (a cast step can be one branch of a
// top-level workflow `type: parallel`/`matrix` step): only the outermost call
// mutates or restores the process env and re-detected UI state, so
// overlapping recordings never clobber or prematurely restore each other.
var (
	castUIEnvMu    sync.Mutex
	castUIEnvDepth int
)

// castUIEnvKeys are the environment variables that influence color/TTY
// detection; only these are captured/restored around a recording.
var castUIEnvKeys = []string{"NO_COLOR", "ATMOS_FORCE_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE", "COLORTERM", "ATMOS_FORCE_TTY"}

// forceRecordingUIEnv makes the current process's themed UI output (step
// headers/footers, toasts, spinners — anything rendered via pkg/ui during the
// recording) match the color/TTY-forcing environment configured for a
// `type: cast` step's recording.
//
// Env vars configured on a cast step (e.g. ATMOS_FORCE_COLOR/ATMOS_FORCE_TTY
// from a `.env.recording` block) are otherwise only ever applied to the child
// subprocess environments of nested `type: shell` steps — pkg/ui's terminal
// detection, performed once at process startup from the real OS environment,
// never sees them. That leaves anything rendered in-process during the
// recording (step labels, toast/spinner messages) unstyled — or the spinner
// forced to its non-interactive fallback — regardless of how the recording's
// env is configured. This applies the same env vars to the current process
// and calls ui.ReinitFormatter to re-run that same detection, so in-process
// output captured into the recording renders identically to the recorded
// subprocess output.
//
// Returns a restore func that reverts both the environment and the
// re-detected UI state; always call it, even on error paths.
func forceRecordingUIEnv(env map[string]string) func() {
	relevant := make(map[string]string, len(castUIEnvKeys))
	for _, key := range castUIEnvKeys {
		if value, ok := env[key]; ok {
			relevant[key] = value
		}
	}
	if len(relevant) == 0 {
		return func() {}
	}

	castUIEnvMu.Lock()
	defer castUIEnvMu.Unlock()
	castUIEnvDepth++
	if castUIEnvDepth > 1 {
		// An outer (or concurrent sibling) cast recording already forced the
		// environment; just track this scope's exit without mutating/restoring.
		return func() {
			castUIEnvMu.Lock()
			defer castUIEnvMu.Unlock()
			castUIEnvDepth--
		}
	}

	priorEnv := make(map[string]*string, len(relevant))
	for key, value := range relevant {
		if old, ok := os.LookupEnv(key); ok {
			oldCopy := old
			priorEnv[key] = &oldCopy
		} else {
			priorEnv[key] = nil
		}
		_ = os.Setenv(key, value)
	}
	ui.ReinitFormatter()

	return func() {
		castUIEnvMu.Lock()
		defer castUIEnvMu.Unlock()
		castUIEnvDepth--
		if castUIEnvDepth > 0 {
			return
		}
		for key, old := range priorEnv {
			if old == nil {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, *old)
			}
		}
		ui.ReinitFormatter()
	}
}

func castRecorderEnv(step *schema.WorkflowStep, vars *Variables) (map[string]string, error) {
	env := make(map[string]string)
	for key, value := range vars.Env {
		env[key] = value
	}
	if len(step.Env) == 0 {
		return env, nil
	}
	resolvedEnv, err := vars.ResolveEnvMap(step.Env)
	if err != nil {
		return nil, fmt.Errorf("step '%s': %w", step.Name, err)
	}
	for key, value := range resolvedEnv {
		env[key] = value
	}
	return env, nil
}

func castRecorderDimensions(step *schema.WorkflowStep) (int, int) {
	width := step.Width
	if width <= 0 {
		width = viper.GetInt("cast.recording.width")
	}
	height := step.Height
	if height <= 0 {
		height = viper.GetInt("cast.recording.height")
	}
	return width, height
}

func applyCastRecordingDefaults(step *schema.WorkflowStep) {
	if step.Defaults == nil || step.Defaults.Cast == nil {
		return
	}
	defaults := step.Defaults.Cast
	if step.Rate == "" {
		step.Rate = defaults.Rate
	}
	if step.Width <= 0 {
		step.Width = defaults.Width
	}
	if step.Height <= 0 {
		step.Height = defaults.Height
	}
}

func runCastBody(ctx context.Context, castStep *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) error {
	switch castMode(castStep) {
	case "steps":
		return runCastStepMode(ctx, castStep, vars, workflow)
	case "session":
		return runCastSessionMode(ctx, castStep, vars, workflow)
	default:
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrInvalidCastMode, castStep.Mode)
	}
}

func runCastStepMode(ctx context.Context, castStep *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) error {
	normalizeCastOutputMode(castStep)
	if err := applyCastStepEnv(castStep, vars); err != nil {
		return err
	}
	executor := NewStepExecutorWithVars(vars)
	if workflow != nil {
		executor.SetWorkflow(workflow)
	}
	runner := castChildStepRunner{ctx: ctx, castStep: castStep, vars: vars, executor: executor, workflow: workflow}
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
	prevWasSession := false
	for i := range castStep.Steps {
		child := &castStep.Steps[i]
		if !child.When.EvaluateWithImplicitSuccess(conditionContext) {
			continue
		}
		prepareCastChildStep(castStep, child, i)
		skipPrompt := prevWasSession && child.Type == schema.TaskTypeSimulate
		if err := runner.run(child, skipPrompt); err != nil {
			fail(err)
		}
		prevWasSession = child.Type == castSessionStepType
	}
	if err := recordCastPromptWithCursor(nil, castStepHasVisibleCursor(castStep)); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

// runCastChildStep executes one child step of a steps-mode cast: simulate
// steps replay scripted output, everything else runs through the executor
// followed by the configured input pause. The skipPrompt flag suppresses a
// simulate step's own prompt draw when a real prompt is already visible (see the
// prevWasSession tracking in runCastStepMode, and castSessionActions' Fn
// wrapper, which always skips it since a live session's real shell prompt is
// always already showing before any of its actions run).
type castChildStepRunner struct {
	ctx      context.Context
	castStep *schema.WorkflowStep
	vars     *Variables
	executor *StepExecutor
	workflow *schema.WorkflowDefinition
}

func (r castChildStepRunner) run(child *schema.WorkflowStep, skipPrompt bool) error {
	switch child.Type {
	case schema.TaskTypeSimulate:
		return runCastSimulateStep(r.ctx, r.castStep, child, r.vars, skipPrompt)
	case castSessionStepType:
		return runCastSessionBlock(r.ctx, r.castStep, child, r.vars, r.workflow)
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

// runCastSessionBlock opens a real PTY session scoped to one steps-mode
// child: session-level config (Shell/WorkingDirectory/Width/Height/
// WriteRate/KeyInterval/Defaults) falls back to the enclosing cast step's
// when unset, exactly like a top-level `mode: session` cast step's config
// -- the block's own `steps:` are its write/key/pause/wait/simulate/exec
// actions, converted via the same castSessionActions used by top-level
// session mode. WorkingDirectory is inherited earlier, by
// prepareCastChildStep, before this runs.
func runCastSessionBlock(ctx context.Context, castStep, block *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) error {
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
		Shell:       block.Shell,
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

func applyCastStepEnv(castStep *schema.WorkflowStep, vars *Variables) error {
	if len(castStep.Env) == 0 {
		return nil
	}
	resolvedEnv, err := vars.ResolveEnvMap(castStep.Env)
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

func runCastSessionMode(ctx context.Context, castStep *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) error {
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
		Shell:       castStep.Shell,
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
// executor (simulate, and any registered step type).
func isCastSessionPTYAction(actionType string) bool {
	switch actionType {
	case "write", "key", "pause", "wait":
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
	})
}

func castMode(step *schema.WorkflowStep) string {
	mode := strings.TrimSpace(step.Mode)
	if mode == "" {
		return "steps"
	}
	return mode
}

func validateCastSessionAction(castStep, action *schema.WorkflowStep) error {
	switch action.Type {
	case "write":
		return validateWriteAction(action)
	case "key":
		return validateKeyAction(action)
	case "pause":
		return validatePauseAction(action)
	case "wait":
		return validateWaitAction(action)
	case schema.TaskTypeSimulate:
		child := *action
		applyCastSimulateDefaults(castStep, &child)
		return validateCastSimulateStep(&child)
	default:
		return validateCastSessionExecAction(action)
	}
}

// validateCastSessionExecAction validates a session action that isn't one of
// the PTY-driving verbs or "simulate": it must name a registered step type
// (e.g. "shell", "atmos", "script"), and that handler's own Validate must
// accept it -- mirroring how mode: steps validates its children.
func validateCastSessionExecAction(action *schema.WorkflowStep) error {
	actionType := action.Type
	if actionType == "" {
		actionType = schema.TaskTypeShell
	}
	handler, ok := Get(actionType)
	if !ok {
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrUnsupportedSessionAction, action.Type)
	}
	return handler.Validate(action)
}

func validateWriteAction(action *schema.WorkflowStep) error {
	if action.Text == "" {
		return ErrWriteActionRequiresText
	}
	if _, err := parseDurationDefault(action.Rate, 0); action.Rate != "" && err != nil {
		return err
	}
	return nil
}

func validateKeyAction(action *schema.WorkflowStep) error {
	if action.Key == "" {
		return ErrKeyActionRequiresKey
	}
	if action.Interval == "" {
		return nil
	}
	if _, err := time.ParseDuration(action.Interval); err != nil {
		return fmt.Errorf("key interval: %w", err)
	}
	return nil
}

func validatePauseAction(action *schema.WorkflowStep) error {
	if action.Duration == "" {
		return ErrPauseActionRequiresDuration
	}
	if _, err := time.ParseDuration(action.Duration); err != nil {
		return fmt.Errorf("pause duration: %w", err)
	}
	return nil
}

func validateWaitAction(action *schema.WorkflowStep) error {
	hasText := action.Text != ""
	hasRegex := action.Regex != ""
	if hasText == hasRegex {
		return ErrWaitActionRequiresTextOrRegex
	}
	if hasRegex {
		if _, err := regexp.Compile(action.Regex); err != nil {
			return fmt.Errorf("wait regex: %w", err)
		}
	}
	if action.Timeout == "" {
		return nil
	}
	if _, err := time.ParseDuration(action.Timeout); err != nil {
		return fmt.Errorf("wait timeout: %w", err)
	}
	return nil
}

func parseDurationDefault(value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	if value == "0" {
		return 0, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", value, err)
	}
	return duration, nil
}
