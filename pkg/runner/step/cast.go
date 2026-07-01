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
)

const wrappedQuotedErrorFormat = "%w: %q"

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
		if len(step.Steps) == 0 {
			return fmt.Errorf(wrappedQuotedErrorFormat, ErrCastStepRequiresSteps, step.Name)
		}
	case "session":
		if len(step.Steps) == 0 {
			return fmt.Errorf(wrappedQuotedErrorFormat, ErrCastSessionRequiresActions, step.Name)
		}
		for i := range step.Steps {
			if err := validateCastSessionAction(&step.Steps[i]); err != nil {
				return fmt.Errorf("cast session action %d: %w", i+1, err)
			}
		}
	default:
		return fmt.Errorf("%w: %q mode %q", ErrInvalidCastMode, step.Name, step.Mode)
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
	executor := NewStepExecutorWithVars(vars)
	if workflow != nil {
		executor.SetWorkflow(workflow)
	}
	for i := range castStep.Steps {
		child := &castStep.Steps[i]
		prepareCastChildStep(castStep, child, i)
		if _, err := executor.Execute(ctx, child); err != nil {
			return err
		}
	}
	return nil
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
