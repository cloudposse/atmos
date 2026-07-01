package step

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

var (
	ErrCastStepRequiresSteps         = errors.New("cast step requires nested steps")
	ErrCastSessionRequiresActions    = errors.New("cast session step requires session actions")
	ErrInvalidCastMode               = errors.New("cast step has invalid mode")
	ErrWriteActionRequiresText       = errors.New("write action requires text")
	ErrKeyActionRequiresKey          = errors.New("key action requires key")
	ErrPauseActionRequiresDuration   = errors.New("pause action requires duration")
	ErrWaitActionRequiresTextOrRegex = errors.New("wait action requires exactly one of text or regex")
	ErrUnsupportedSessionAction      = errors.New("unsupported session action type")
	ErrInvalidSimulateMode           = errors.New("simulate step has invalid mode")
	ErrSimulateTypedRequiresText     = errors.New("simulate typed step requires text")
	ErrUnsupportedPromptStyle        = errors.New("unsupported simulate prompt style")
)

const wrappedQuotedErrorFormat = "%w: %q"

const (
	defaultCastTypeDelay      = 35 * time.Millisecond
	defaultCastPromptDelay    = 350 * time.Millisecond
	defaultCastEnterDelay     = 550 * time.Millisecond
	defaultCastStepPauseDelay = 700 * time.Millisecond
)

type CastHandler struct {
	BaseHandler
}

func init() {
	Register(&CastHandler{
		BaseHandler: NewBaseHandler(schema.TaskTypeCast, CategoryCommand, false),
	})
}

func (h *CastHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.CastHandler.Validate")()

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
	for i := range step.Steps {
		if step.Steps[i].Type == schema.TaskTypeSimulate {
			if err := validateCastSimulateStep(&step.Steps[i]); err != nil {
				return fmt.Errorf("cast simulate step %d: %w", i+1, err)
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
		if err := validateCastSessionAction(&step.Steps[i]); err != nil {
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

	rec, restore, err := startStepRecorder(step)
	if err != nil {
		return nil, err
	}
	runErr := runCastBody(ctx, step, vars, workflow)
	restore()
	closeErr := rec.Close()
	if runErr == nil && closeErr != nil {
		runErr = closeErr
	}
	if closeErr == nil {
		_, _ = fmt.Fprintf(iolib.GetContext().UI(), "Cast recorded: %s\n", rec.Path())
	}
	if runErr == nil {
		runErr = renderCastOutputs(step, rec.Path())
	}
	result := NewStepResult(rec.Path()).WithMetadata("cast", rec.Path())
	if step.CastOutput != nil {
		result.WithMetadata("svg", step.CastOutput.SVG)
		result.WithMetadata("gif", step.CastOutput.GIF)
		result.WithMetadata("mp4", step.CastOutput.MP4)
	}
	return result, runErr
}

func startStepRecorder(step *schema.WorkflowStep) (*asciicast.Recorder, func(), error) {
	path := ""
	if step.CastOutput != nil {
		path = step.CastOutput.Cast
	}
	env := make(map[string]string)
	for _, pair := range os.Environ() {
		k, v, ok := strings.Cut(pair, "=")
		if ok {
			env[k] = v
		}
	}
	width := step.Width
	if width <= 0 {
		width = viper.GetInt("cast.recording.width")
	}
	height := step.Height
	if height <= 0 {
		height = viper.GetInt("cast.recording.height")
	}
	outputRate, err := parseDurationDefault(step.Rate, 0)
	if err != nil {
		return nil, nil, err
	}
	rec, err := asciicast.Start(&asciicast.Options{
		Path:       path,
		BasePath:   viper.GetString("cast.recording.base_path"),
		Command:    castCommandArgs(step),
		Width:      width,
		Height:     height,
		RecordIn:   viper.GetBool("cast.recording.input"),
		Explicit:   path != "",
		Env:        env,
		OutputRate: outputRate,
	})
	if err != nil {
		return nil, nil, err
	}
	return rec, iolib.SetRecorder(rec), nil
}

func runCastBody(ctx context.Context, castStep *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) error {
	switch castMode(castStep) {
	case "steps":
		return runCastStepMode(ctx, castStep, vars, workflow)
	case "session":
		return runCastSessionMode(ctx, castStep)
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
	conditionContext := schema.ConditionContext{Status: schema.ConditionPredicateSuccess}
	var runErr error
	for i := range castStep.Steps {
		child := &castStep.Steps[i]
		if !child.When.EvaluateWithImplicitSuccess(conditionContext) {
			continue
		}
		prepareCastChildStep(castStep, child, i)
		if child.Type == schema.TaskTypeSimulate {
			if err := runCastSimulateStep(ctx, castStep, child, vars); err != nil {
				if runErr == nil {
					runErr = err
				} else {
					runErr = errors.Join(runErr, err)
				}
				conditionContext.Status = schema.ConditionPredicateFailure
			}
			continue
		}
		if _, err := executor.Execute(ctx, child); err != nil {
			if runErr == nil {
				runErr = err
			} else {
				runErr = errors.Join(runErr, err)
			}
			conditionContext.Status = schema.ConditionPredicateFailure
			continue
		}
		if err := sleepCastInput(ctx, castStepPauseDelay(child)); err != nil {
			if runErr == nil {
				runErr = err
			} else {
				runErr = errors.Join(runErr, err)
			}
			conditionContext.Status = schema.ConditionPredicateFailure
		}
	}
	if err := recordCastPrompt(nil); err != nil {
		if runErr == nil {
			return err
		}
		return errors.Join(runErr, err)
	}
	return runErr
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
}

func runCastSimulateStep(ctx context.Context, castStep, child *schema.WorkflowStep, vars *Variables) error {
	switch castSimulateMode(child) {
	case "typed":
		text, err := vars.Resolve(strings.TrimRight(child.Text, "\n"))
		if err != nil {
			return err
		}
		writeRate, err := parseDurationDefault(firstNonEmpty(child.Rate, castStep.WriteRate), defaultCastTypeDelay)
		if err != nil {
			return err
		}
		enterDelay, err := castStepEnterDelay(child)
		if err != nil {
			return err
		}
		return recordCastTypedLine(ctx, child.SimulatePrompt, text, writeRate, enterDelay)
	case "prompt":
		return recordCastPrompt(child.SimulatePrompt)
	default:
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrInvalidSimulateMode, child.Mode)
	}
}

func recordCastTypedLine(ctx context.Context, prompt *schema.SimulatePrompt, line string, writeRate, enterDelay time.Duration) error {
	if err := recordCastPrompt(prompt); err != nil {
		return err
	}
	if err := sleepCastInput(ctx, defaultCastPromptDelay); err != nil {
		return err
	}
	for _, char := range line {
		if err := sleepCastInput(ctx, writeRate); err != nil {
			return err
		}
		if _, err := fmt.Fprint(iolib.GetContext().Data(), string(char)); err != nil {
			return err
		}
	}
	if err := sleepCastInput(ctx, enterDelay); err != nil {
		return err
	}
	_, err := fmt.Fprint(iolib.GetContext().Data(), "\n")
	return err
}

func castPromptText(prompt *schema.SimulatePrompt) string {
	if prompt == nil || prompt.Text == "" {
		return "> "
	}
	return prompt.Text
}

func castPromptStyle(prompt *schema.SimulatePrompt) string {
	if prompt == nil || strings.TrimSpace(prompt.Style) == "" {
		return "command"
	}
	return strings.TrimSpace(prompt.Style)
}

func renderCastPrompt(prompt *schema.SimulatePrompt) (string, error) {
	text := castPromptText(prompt)
	styles := theme.GetCurrentStyles()
	if styles == nil {
		return text, nil
	}
	switch castPromptStyle(prompt) {
	case "command":
		return styles.Command.Bold(true).Render(text), nil
	case "label":
		return styles.Label.Render(text), nil
	case "muted":
		return styles.Muted.Render(text), nil
	case "info":
		return styles.Info.Render(text), nil
	case "notice":
		return styles.Notice.Render(text), nil
	default:
		return "", fmt.Errorf(wrappedQuotedErrorFormat, ErrUnsupportedPromptStyle, castPromptStyle(prompt))
	}
}

func validateCastSimulateStep(step *schema.WorkflowStep) error {
	switch castSimulateMode(step) {
	case "typed":
		if strings.TrimRight(step.Text, "\n") == "" {
			return ErrSimulateTypedRequiresText
		}
		if _, err := parseDurationDefault(step.Rate, 0); step.Rate != "" && err != nil {
			return err
		}
		if _, err := castStepEnterDelay(step); err != nil {
			return err
		}
	case "prompt":
	default:
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrInvalidSimulateMode, step.Mode)
	}
	_, err := renderCastPrompt(step.SimulatePrompt)
	return err
}

func castSimulateMode(step *schema.WorkflowStep) string {
	mode := strings.TrimSpace(step.Mode)
	if mode == "" {
		return "typed"
	}
	return mode
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func recordCastPrompt(prompt *schema.SimulatePrompt) error {
	rendered, err := renderCastPrompt(prompt)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(iolib.GetContext().Data(), rendered)
	return err
}

func castStepEnterDelay(child *schema.WorkflowStep) (time.Duration, error) {
	return parseDurationDefault(child.Duration, defaultCastEnterDelay)
}

func castStepPauseDelay(child *schema.WorkflowStep) time.Duration {
	delay, err := parseDurationDefault(child.Interval, defaultCastStepPauseDelay)
	if err != nil {
		return defaultCastStepPauseDelay
	}
	return delay
}

func sleepCastInput(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runCastSessionMode(ctx context.Context, castStep *schema.WorkflowStep) error {
	writeRate, err := parseDurationDefault(castStep.WriteRate, 40*time.Millisecond)
	if err != nil {
		return err
	}
	keyInterval, err := parseDurationDefault(castStep.KeyInterval, 20*time.Millisecond)
	if err != nil {
		return err
	}
	return asciicast.RunSession(ctx, asciicast.SessionOptions{
		Shell:       castStep.Shell,
		Width:       castStep.Width,
		Height:      castStep.Height,
		WriteRate:   writeRate,
		KeyInterval: keyInterval,
		Actions:     castSessionActions(castStep.Steps),
	})
}

func castSessionActions(steps []schema.WorkflowStep) []asciicast.SessionAction {
	actions := make([]asciicast.SessionAction, 0, len(steps))
	for i := range steps {
		child := &steps[i]
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
	}
	return actions
}

func castCommandArgs(step *schema.WorkflowStep) []string {
	command := strings.TrimSpace(step.Command)
	if command != "" {
		return strings.Fields(command)
	}
	return []string{"cast", step.Name}
}

func renderCastOutputs(step *schema.WorkflowStep, castPath string) error {
	if step.CastOutput == nil {
		return nil
	}
	return asciicast.Render(castPath, asciicast.RenderOptions{
		SVG: step.CastOutput.SVG,
		GIF: step.CastOutput.GIF,
		MP4: step.CastOutput.MP4,
	})
}

func castMode(step *schema.WorkflowStep) string {
	mode := strings.TrimSpace(step.Mode)
	if mode == "" {
		return "steps"
	}
	return mode
}

func validateCastSessionAction(action *schema.WorkflowStep) error {
	switch action.Type {
	case "write":
		return validateWriteAction(action)
	case "key":
		return validateKeyAction(action)
	case "pause":
		return validatePauseAction(action)
	case "wait":
		return validateWaitAction(action)
	default:
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrUnsupportedSessionAction, action.Type)
	}
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
	_, err := time.ParseDuration(action.Interval)
	return err
}

func validatePauseAction(action *schema.WorkflowStep) error {
	if action.Duration == "" {
		return ErrPauseActionRequiresDuration
	}
	_, err := time.ParseDuration(action.Duration)
	return err
}

func validateWaitAction(action *schema.WorkflowStep) error {
	hasText := action.Text != ""
	hasRegex := action.Regex != ""
	if hasText == hasRegex {
		return ErrWaitActionRequiresTextOrRegex
	}
	if hasRegex {
		if _, err := regexp.Compile(action.Regex); err != nil {
			return err
		}
	}
	if action.Timeout == "" {
		return nil
	}
	_, err := time.ParseDuration(action.Timeout)
	return err
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
