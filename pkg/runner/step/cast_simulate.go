package step

// Simulate-step support for steps-mode casts: scripted prompts, typed text
// with deterministic jitter, and cursor/prompt rendering used to make
// recordings look like a real interactive session.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/muesli/termenv"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func applyCastSimulateDefaults(castStep, child *schema.WorkflowStep) {
	if castStep.Defaults == nil || castStep.Defaults.Simulate == nil || child.Type != schema.TaskTypeSimulate {
		return
	}
	defaults := castStep.Defaults.Simulate
	if child.Mode == "" {
		child.Mode = defaults.Mode
	}
	if !child.CursorSet && defaults.Cursor != nil {
		child.Cursor = *defaults.Cursor
		child.CursorSet = true
	}
	applyCastSimulatePromptDefault(defaults.Prompt, child)
	applyCastSimulateTimingDefaults(defaults, child)
}

func applyCastSimulatePromptDefault(prompt *schema.SimulatePrompt, child *schema.WorkflowStep) {
	if child.SimulatePrompt == nil {
		child.SimulatePrompt = cloneSimulatePrompt(prompt)
		return
	}
	if prompt == nil {
		return
	}
	if child.SimulatePrompt.Text == "" {
		child.SimulatePrompt.Text = prompt.Text
	}
	if child.SimulatePrompt.Style == "" {
		child.SimulatePrompt.Style = prompt.Style
	}
}

func applyCastSimulateTimingDefaults(defaults *schema.CastSimulateDefaults, child *schema.WorkflowStep) {
	if child.Rate == "" {
		child.Rate = defaults.Rate
	}
	if child.Jitter == 0 {
		child.Jitter = defaults.Jitter
	}
	if child.Duration == "" {
		child.Duration = defaults.Duration
	}
	if child.Interval == "" {
		child.Interval = defaults.Interval
	}
}

func cloneSimulatePrompt(prompt *schema.SimulatePrompt) *schema.SimulatePrompt {
	if prompt == nil {
		return nil
	}
	clone := *prompt
	return &clone
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
		jitter := firstNonZeroFloat(child.Jitter, castStep.Jitter)
		enterDelay, err := castStepEnterDelay(child)
		if err != nil {
			return err
		}
		return recordCastTypedLine(ctx, castTypedLineOptions{
			Prompt:     child.SimulatePrompt,
			Line:       text,
			WriteRate:  writeRate,
			Jitter:     jitter,
			EnterDelay: enterDelay,
			Cursor:     child.Cursor,
		})
	case "prompt":
		return recordCastPromptWithCursor(child.SimulatePrompt, child.Cursor)
	default:
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrInvalidSimulateMode, child.Mode)
	}
}

func recordCastTypedLine(ctx context.Context, opts castTypedLineOptions) error {
	if err := recordCastPromptWithCursor(opts.Prompt, opts.Cursor); err != nil {
		return err
	}
	if err := sleepCastInput(ctx, defaultCastPromptDelay); err != nil {
		return err
	}
	if err := recordCastTypedText(ctx, opts.Prompt, opts.Line, opts.WriteRate, opts.Jitter); err != nil {
		return err
	}
	if err := sleepCastInput(ctx, opts.EnterDelay); err != nil {
		return err
	}
	_, err := fmt.Fprint(iolib.GetContext().Data(), "\n")
	return err
}

func recordCastTypedText(ctx context.Context, prompt *schema.SimulatePrompt, line string, writeRate time.Duration, jitter float64) error {
	stylePrefix, styleSuffix, err := renderCastTypedLineParts(prompt, line)
	if err != nil {
		return err
	}
	if stylePrefix != "" {
		if _, err := fmt.Fprint(iolib.GetContext().Data(), stylePrefix); err != nil {
			return err
		}
	}
	chars := []rune(line)
	for i, char := range chars {
		if err := sleepCastInput(ctx, castTypedCharDelay(line, chars, i, writeRate, jitter)); err != nil {
			return err
		}
		if _, err := fmt.Fprint(iolib.GetContext().Data(), string(char)); err != nil {
			return err
		}
	}
	if styleSuffix == "" {
		return nil
	}
	_, err = fmt.Fprint(iolib.GetContext().Data(), styleSuffix)
	return err
}

func castTypedCharDelay(line string, chars []rune, index int, baseDelay time.Duration, jitter float64) time.Duration {
	if baseDelay <= 0 || jitter <= 0 {
		return baseDelay
	}

	unit := deterministicCastJitterUnit(line, index)
	factor := 1 - jitter + (2 * jitter * unit)
	if index > 0 {
		switch prev := chars[index-1]; {
		case prev == ' ' || prev == '\t':
			factor *= castWhitespaceBoundaryMinFactor + (castWhitespaceBoundaryJitterFactor * unit)
		case strings.ContainsRune("|&;=,:/", prev):
			factor *= castPunctuationBoundaryMinFactor + (castPunctuationBoundaryJitterFactor * unit)
		}
	}
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		factor *= castCommentTypingFactor
	}

	return time.Duration(float64(baseDelay) * factor)
}

func deterministicCastJitterUnit(line string, index int) float64 {
	hash := castJitterHashOffset
	for _, char := range line {
		hash ^= uint64(char)
		hash *= castJitterHashPrime
	}
	for _, char := range strconv.Itoa(index) {
		hash ^= uint64(char)
		hash *= castJitterHashPrime
	}
	return float64(hash%castJitterScale) / float64(castJitterScaleMax)
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
	return renderCastStyledText(castPromptText(prompt), castPromptStyle(prompt), true)
}

func castTypedTextStyle(_ *schema.SimulatePrompt, line string) string {
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return "muted"
	}
	return "body"
}

func renderCastTypedText(prompt *schema.SimulatePrompt, line, text string) (string, error) {
	return renderCastStyledText(text, castTypedTextStyle(prompt, line), false)
}

func renderCastTypedLineParts(prompt *schema.SimulatePrompt, line string) (string, string, error) {
	rendered, err := renderCastTypedText(prompt, line, line)
	if err != nil {
		return "", "", err
	}
	if rendered == line {
		return "", "", nil
	}
	index := strings.Index(rendered, line)
	if index == -1 {
		return "", "", nil
	}
	return rendered[:index], rendered[index+len(line):], nil
}

// castStyleMu serializes the global color-profile force/restore in
// renderCastStyledText: concurrent cast branches (e.g. control steps with
// MaxConcurrency) would otherwise interleave the toggles and restore the
// wrong profile, corrupting generated ANSI output.
var castStyleMu sync.Mutex

func renderCastStyledText(text, styleName string, bold bool) (string, error) {
	castStyleMu.Lock()
	defer castStyleMu.Unlock()

	restoreColorProfile := forceCastColorProfile()
	defer restoreColorProfile()

	styles := theme.GetCurrentStyles()
	if styles == nil {
		return text, nil
	}
	switch styleName {
	case "body":
		return styles.Body.Render(text), nil
	case "command":
		style := styles.Command
		if bold {
			style = style.Bold(true)
		}
		return style.Render(text), nil
	case "label":
		return styles.Label.Render(text), nil
	case "muted":
		return styles.Muted.Render(text), nil
	case "info":
		return styles.Info.Render(text), nil
	case "notice":
		return styles.Notice.Render(text), nil
	default:
		return "", fmt.Errorf(wrappedQuotedErrorFormat, ErrUnsupportedPromptStyle, styleName)
	}
}

func forceCastColorProfile() func() {
	profile := ui.GetColorProfile()
	if profile == termenv.TrueColor {
		return func() {}
	}
	ui.SetColorProfile(termenv.TrueColor)
	return func() {
		ui.SetColorProfile(profile)
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
		if step.Jitter < 0 || step.Jitter > 1 {
			return fmt.Errorf("%w: %v", ErrInvalidSimulateJitter, step.Jitter)
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

func firstNonZeroFloat(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func recordCastPrompt(prompt *schema.SimulatePrompt) error {
	rendered, err := renderCastPrompt(prompt)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(iolib.GetContext().Data(), rendered)
	return err
}

func recordCastPromptWithCursor(prompt *schema.SimulatePrompt, cursor bool) error {
	rendered, err := renderCastPrompt(prompt)
	if err != nil {
		return err
	}
	if cursor {
		rendered += castCursorShow
	}
	_, err = fmt.Fprint(iolib.GetContext().Data(), rendered)
	return err
}

func castStepHasVisibleCursor(step *schema.WorkflowStep) bool {
	for i := range step.Steps {
		child := &step.Steps[i]
		if child.Type == schema.TaskTypeSimulate && child.Cursor {
			return true
		}
	}
	return false
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
