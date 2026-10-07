package step

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

const maxAutomationDepth = 64

// AutomationLibrary exposes the existing registry to Go callers and embedded languages.
// State is owned by an invocation; Fork snapshots it for a parallel task.
type AutomationLibrary struct {
	vars           *Variables
	workflow       *schema.WorkflowDefinition
	processChanges map[string]string
}

// NewAutomationLibrary snapshots the caller's context without letting a script
// mutate its enclosing workflow, command, or hook variables.
func NewAutomationLibrary(vars *Variables, workflow *schema.WorkflowDefinition) *AutomationLibrary {
	defer perf.Track(nil, "step.NewAutomationLibrary")()
	if vars == nil {
		vars = NewVariables()
	}
	snapshot := vars.Clone()
	snapshot.automationDepth++
	return &AutomationLibrary{vars: snapshot, workflow: workflow, processChanges: map[string]string{}}
}

// Names includes canonical names and aliases from the live registry.
func (l *AutomationLibrary) Names() []string {
	defer perf.Track(nil, "step.AutomationLibrary.Names")()
	names := []string{}
	for name, handler := range List() {
		names = append(names, name)
		if aliases, ok := handler.(aliasedHandler); ok {
			names = append(names, aliases.GetAliases()...)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// Fork isolates mutable variables for an invocation or concurrent task.
func (l *AutomationLibrary) Fork() automation.StepLibrary {
	defer perf.Track(nil, "step.AutomationLibrary.Fork")()
	return &AutomationLibrary{vars: l.vars.Clone(), workflow: l.workflow, processChanges: maps.Clone(l.processChanges)}
}

// Run validates configuration and dispatches through the shared step executor.
func (l *AutomationLibrary) Run(ctx context.Context, call *automation.StepCall) (*automation.StepResult, error) {
	defer perf.Track(nil, "step.AutomationLibrary.Run")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if l.vars.automationDepth > maxAutomationDepth {
		return nil, fmt.Errorf("%w: automation step nesting exceeds 64 levels", errUtils.ErrAutomation)
	}
	step, err := decodeAutomationStep(call)
	if err != nil {
		return nil, err
	}
	return l.runStep(ctx, step, call)
}

func (l *AutomationLibrary) runStep(ctx context.Context, step *schema.WorkflowStep, call *automation.StepCall) (*automation.StepResult, error) {
	if err := validateAutomationStep(step, call.Parallel || l.vars.automationParallel); err != nil {
		return nil, err
	}
	if err := l.prepareCall(step, call); err != nil {
		return nil, err
	}
	ctx, cancel, err := l.automationContext(ctx, step)
	if err != nil {
		return nil, err
	}
	defer cancel()
	before := maps.Clone(l.vars.Env)
	result, err := l.execute(ctx, step)
	l.rememberEnvironment(before)

	if err != nil {
		return nil, err
	}
	if result == nil {
		return &automation.StepResult{}, nil
	}
	return &automation.StepResult{
		Value: result.Value, Values: slices.Clone(result.Values), Metadata: maps.Clone(result.Metadata),
		Outputs: maps.Clone(result.Outputs), Skipped: result.Skipped, Error: result.Error,
	}, nil
}

func (l *AutomationLibrary) execute(ctx context.Context, step *schema.WorkflowStep) (*StepResult, error) {
	executor := NewStepExecutorWithVars(l.vars)
	executor.SetWorkflow(l.workflow)
	var result *StepResult
	retryConfig := step.Retry
	handler, _ := Get(step.Type)
	if handler.GetName() == "http" {
		retryConfig = nil
	}
	err := retry.WithPredicate(ctx, retryConfig, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var runErr error
		result, runErr = executor.Execute(ctx, step)
		return runErr
	}, func(err error) bool {
		return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, errUtils.ErrWorkflowExit) && !errors.Is(err, errUtils.ErrUserAborted)
	})
	return result, err
}

func (l *AutomationLibrary) rememberEnvironment(before map[string]string) {
	for key, value := range l.vars.Env {
		if previous, exists := before[key]; !exists || previous != value {
			l.processChanges[key] = value
		}
	}
}

func (l *AutomationLibrary) prepareCall(step *schema.WorkflowStep, call *automation.StepCall) error {
	if call.ProcessEnv != nil {
		l.vars.Env = envpkg.SliceToMap(call.ProcessEnv)
	}
	if l.vars.Env == nil {
		l.vars.Env = map[string]string{}
	}
	maps.Copy(l.vars.Env, l.processChanges)
	l.vars.automationParallel = call.Parallel || l.vars.automationParallel
	l.vars.OutputWriters = OutputWriters{Stdout: call.Stdout, Stderr: call.Stderr}
	dir, err := l.vars.Resolve(step.WorkingDirectory)
	if err != nil {
		return err
	}
	if dir == "" {
		dir = call.WorkingDirectory
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(call.WorkingDirectory, dir)
	}
	step.WorkingDirectory = dir
	return nil
}

func decodeAutomationStep(call *automation.StepCall) (*schema.WorkflowStep, error) {
	if call == nil {
		return nil, fmt.Errorf("%w: step request is required", errUtils.ErrAutomation)
	}
	fields := call.Configuration
	if fields == nil {
		fields = map[string]any{}
	}
	var node yaml.Node
	if err := node.Encode(fields); err != nil {
		return nil, fmt.Errorf("%w: step configuration: %w", errUtils.ErrAutomation, err)
	}
	if err := validateAutomationStepFields(&node, call.Type); err != nil {
		return nil, err
	}
	mapping := &node
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "type"}, &yaml.Node{Kind: yaml.ScalarNode, Value: call.Type})
	var step schema.WorkflowStep
	// Decode through the YAML schema so polymorphic with/output/background fields
	// retain exactly the same meaning as they have in workflow files.
	if err := node.Decode(&step); err != nil {
		return nil, fmt.Errorf("%w: step configuration: %w", errUtils.ErrAutomation, err)
	}
	step.Type = call.Type
	if step.Name == "" {
		step.Name = call.Type
	}
	return &step, nil
}

// Validate checks a Go request without executing a handler or mutating state.
func (l *AutomationLibrary) Validate(call *automation.StepCall) error {
	defer perf.Track(nil, "step.AutomationLibrary.Validate")()
	step, err := decodeAutomationStep(call)
	if err != nil {
		return err
	}
	return validateAutomationStep(step, call.Parallel || l.vars.automationParallel)
}
