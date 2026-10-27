package step

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/castplan"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const declaredTypeField = "type"

var _ automation.CastLibrary = (*AutomationLibrary)(nil)

// ValidateCast checks every action, including setup and cleanup, without executing anything.
func (l *AutomationLibrary) ValidateCast(plan *castplan.Plan, call *automation.StepCall) error {
	defer perf.Track(nil, "step.AutomationLibrary.ValidateCast")()
	_, err := l.prepareCast(plan, call)
	return err
}

// GenerateCast executes a declaration with guaranteed cleanup and validates before publishing.
func (l *AutomationLibrary) GenerateCast(ctx context.Context, plan *castplan.Plan, call *automation.StepCall, validate func(string) error) (string, error) {
	defer perf.Track(nil, "step.AutomationLibrary.GenerateCast")()
	run, err := l.prepareCast(plan, call)
	if err != nil {
		return "", err
	}
	deadline, err := l.automationDeadline(ctx, run.step)
	if err != nil {
		return "", err
	}
	defer deadline.Stop()
	run.ctx = deadline.Context()
	return run.generate(plan, validate)
}

type declaredCast struct {
	ctx             context.Context
	step            *schema.WorkflowStep
	vars            *Variables
	call            *automation.StepCall
	library         *AutomationLibrary
	recorder        *asciicast.Recorder
	previousSession bool
}

func (l *AutomationLibrary) prepareCast(plan *castplan.Plan, call *automation.StepCall) (*declaredCast, error) {
	step, err := decodeCastPlan(plan, call)
	if err != nil {
		return nil, err
	}
	if err := validateAutomationStep(step, call.Parallel || l.vars.automationParallel); err != nil {
		return nil, err
	}

	library := l.Fork().(*AutomationLibrary)
	if err := library.prepareCall(step, call); err != nil {
		return nil, err
	}
	if err := prepareDeclaredCastOptions(library, step, call); err != nil {
		return nil, err
	}

	run := &declaredCast{step: step, vars: library.vars, call: call, library: library}
	for _, actions := range [][]castplan.Action{plan.Setup, plan.Actions, plan.Cleanup} {
		if err := run.validateActions(actions); err != nil {
			return nil, err
		}
	}
	return run, nil
}

func decodeCastPlan(plan *castplan.Plan, call *automation.StepCall) (*schema.WorkflowStep, error) {
	if plan == nil || call == nil {
		return nil, castDeclarationError("recording declaration and invocation are required")
	}
	config := maps.Clone(plan.Configuration)
	if config == nil {
		config = map[string]any{}
	}
	// The existing cast handler validates settings against a harmless presentation action.
	config["steps"] = []any{map[string]any{declaredTypeField: "simulate", "mode": "prompt"}}
	request := *call
	request.Configuration = config
	return decodeAutomationStep(&request)
}

func prepareDeclaredCastOptions(library *AutomationLibrary, step *schema.WorkflowStep, call *automation.StepCall) error {
	if step.CastOutput == nil || step.CastOutput.Cast == "" {
		return castDeclarationError("cast.recorder requires output.cast (or a string output path)")
	}
	resolveDeclaredOutputs(step.CastOutput, call.WorkingDirectory)
	if step.CastOutput.Mode == "" {
		step.CastOutput.Mode = "raw"
	}
	if step.Show == nil {
		step.Show = &schema.ShowConfig{Labels: new(false)}
	}
	normalizeCastOutputMode(step)
	// Cast output uses the masked recording streams rather than script-result capture.
	library.vars.OutputWriters = OutputWriters{}
	return applyCastStepEnv(step, library.vars)
}

func resolveDeclaredOutputs(output *schema.CastOutput, base string) {
	for _, path := range []*string{&output.Cast, &output.GIF, &output.MP4, &output.HTML, &output.ASCII, &output.PNG, &output.JPG} {
		if *path != "" && !filepath.IsAbs(*path) {
			*path = filepath.Join(base, *path)
		}
	}
}

func (r *declaredCast) validateActions(actions []castplan.Action) error {
	for i := range actions {
		action := &actions[i]
		if action.Kind == "hidden" {
			if err := r.validateActions(action.Children); err != nil {
				return err
			}
			continue
		}
		child, err := r.child(action)
		if err != nil {
			return err
		}
		if action.Kind == "session" {
			err = r.validateTerminal(action, child)
		} else {
			err = r.validateChild(action, child)
		}
		if err != nil {
			return fmt.Errorf("cast action %d (%s): %w", i+1, action.Kind, err)
		}
	}
	return nil
}

func (r *declaredCast) validateChild(action *castplan.Action, child *schema.WorkflowStep) error {
	switch action.Kind {
	case "simulate":
		return validateCastSimulateStep(child)
	case "screenshot":
		if child.Path == "" {
			return ErrScreenshotActionRequiresPath
		}
		return nil
	case "pause":
		return validatePauseAction(child)
	case "run", "step":
		return r.validateExecutable(action, child)
	default:
		return castDeclarationError("unknown action %q", action.Kind)
	}
}

func (r *declaredCast) validateExecutable(action *castplan.Action, child *schema.WorkflowStep) error {
	if child.Type == "cast" {
		return castDeclarationError("nested recordings are not supported")
	}
	if err := validateAutomationStep(child, false); err != nil {
		return err
	}
	if action.Kind == "run" {
		typed, err := r.typing(action, child)
		if err != nil {
			return err
		}
		return validateCastSimulateStep(typed)
	}
	return nil
}

func (r *declaredCast) validateTerminal(action *castplan.Action, child *schema.WorkflowStep) error {
	if len(action.Children) == 0 {
		return ErrCastSessionRequiresActions
	}
	for i := range action.Children {
		item, err := decodeDeclaredAction(&action.Children[i])
		if err != nil {
			return err
		}
		if item.Type == "key" {
			if err := asciicast.ValidateKey(item.Key); err != nil {
				return err
			}
		}
		if err := validateCastSessionAction(child, item); err != nil {
			return err
		}
	}
	return nil
}

func decodeDeclaredAction(action *castplan.Action) (*schema.WorkflowStep, error) {
	fields := maps.Clone(action.Fields)
	if fields == nil {
		fields = map[string]any{}
	}
	kind := action.Kind
	if kind == "run" {
		kind = "shell"
		delete(fields, "display")
		delete(fields, "prompt")
	}
	if kind == "step" {
		kind, _ = fields[declaredTypeField].(string)
		delete(fields, declaredTypeField)
	}
	fields[declaredTypeField] = kind
	var node yaml.Node
	if err := node.Encode(fields); err != nil {
		return nil, err
	}
	// Cast actions also accept timing inherited from their enclosing recorder.
	if err := validateDeclaredFields(fields, kind); err != nil {
		return nil, err
	}
	var step schema.WorkflowStep
	if err := node.Decode(&step); err != nil {
		return nil, err
	}
	return &step, nil
}

func validateDeclaredFields(fields map[string]any, kind string) error {
	allowed, _ := automationFieldsFor(kind)
	switch kind {
	case "write", "key", "wait", "pause", "hide", "show", "session", "screenshot":
		allowed = automationStepFields()
	}
	for _, key := range []string{declaredTypeField, "duration", "interval", "rate", "cursor", "jitter", "prompt"} {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			return castDeclarationError("unknown %s action field %q", kind, key)
		}
	}
	return nil
}

func (r *declaredCast) child(action *castplan.Action) (*schema.WorkflowStep, error) {
	child, err := decodeDeclaredAction(action)
	if err != nil {
		return nil, err
	}
	if child.WorkingDirectory != "" && !filepath.IsAbs(child.WorkingDirectory) {
		child.WorkingDirectory = filepath.Join(r.call.WorkingDirectory, child.WorkingDirectory)
	}
	prepareCastChildStep(r.step, child, 0)
	if action.Kind == "session" {
		inheritCastSessionBlockDefaults(r.step, child)
	}
	return child, nil
}

func castDeclarationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUtils.ErrAutomation, fmt.Sprintf(format, args...))
}

func (r *declaredCast) generate(plan *castplan.Plan, validate func(string) error) (string, error) {
	// Cleanup is armed before setup so partial setup failures still restore fixtures.
	bodyErr := r.unrecorded(plan.Setup, false)
	var restore func()
	if bodyErr == nil {
		r.recorder, restore, bodyErr = startStepRecorder(r.step, r.vars)
	}
	if restore != nil {
		restore = sync.OnceFunc(restore)
		defer restore()
	}
	if r.recorder != nil {
		defer func() { _ = r.recorder.Discard() }()
	}
	if bodyErr == nil {
		bodyErr = runCastTapeRequirements(r.ctx, r.step, r.vars)
	}
	if bodyErr == nil {
		bodyErr = r.recordPlan(plan)
	}
	cleanupErr := r.cleanup(plan.Cleanup)
	if err := errors.Join(bodyErr, cleanupErr); err != nil {
		return "", err
	}
	restore()
	return r.publish(validate)
}

func (r *declaredCast) publish(validate func(string) error) (string, error) {
	if err := r.ctx.Err(); err != nil {
		return "", err
	}
	if validate == nil {
		return "", castDeclarationError("a recording validator is required")
	}
	if err := r.recorder.ValidateBeforeCommit(validate); err != nil {
		return "", err
	}
	if err := renderCastOutputs(r.step, r.recorder.Path()); err != nil {
		return "", err
	}
	if err := asciicast.RenderMarkerScreenshotsAt(r.recorder.Path(), r.call.WorkingDirectory); err != nil {
		return "", err
	}
	return r.recorder.Path(), nil
}

func (r *declaredCast) cleanup(actions []castplan.Action) error {
	original := r.ctx
	ctx, cancel := context.WithTimeout(context.WithoutCancel(original), 30*time.Second)
	defer cancel()
	r.ctx = ctx
	defer func() { r.ctx = original }()
	return r.unrecorded(actions, true)
}

func (r *declaredCast) recordPlan(plan *castplan.Plan) error {
	hash, err := plan.Fingerprint()
	if err != nil {
		return err
	}
	r.marker(castplan.Marker{Kind: "plan", Fingerprint: hash})
	for i := range plan.Actions {
		if err := r.action(&plan.Actions[i], fmt.Sprint(i), true); err != nil {
			return err
		}
	}
	if !r.previousSession {
		if err := recordCastPromptWithCursor(nil, castStepHasVisibleCursor(r.step)); err != nil {
			return err
		}
	}
	r.marker(castplan.Marker{Kind: "complete", Fingerprint: hash})
	return nil
}

func (r *declaredCast) unrecorded(actions []castplan.Action, all bool) error {
	previousSession := r.previousSession
	defer func() { r.previousSession = previousSession }()
	restore := iolib.SetRecorder(nil)
	defer restore()
	var result error
	for i := range actions {
		if err := r.action(&actions[i], "", false); err != nil {
			result = errors.Join(result, err)
			if !all {
				break
			}
		}
	}
	return result
}

func (r *declaredCast) marker(marker castplan.Marker) { iolib.RecordMarker(marker.Encode()) }
