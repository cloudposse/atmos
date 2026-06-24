package step

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	containerStepType          = "container"
	containerActionBuild       = "build"
	containerActionPush        = "push"
	containerActionRun         = "run"
	containerActionInspect     = "inspect"
	containerBuildEngineBuildx = "buildx"
	defaultContainerShell      = "/bin/sh"
	defaultContainerWorkdir    = "/workspace"
)

// ContainerHandler executes one-shot container steps.
type ContainerHandler struct {
	BaseHandler
}

func init() {
	Register(&ContainerHandler{
		BaseHandler: NewBaseHandler(containerStepType, CategoryCommand, false),
	})
}

// Validate checks container step configuration.
func (h *ContainerHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ContainerHandler.Validate")()

	action := containerStepAction(step)
	switch action {
	case containerActionBuild:
		return validateBuildAction(step)
	case containerActionPush:
		return h.validatePushAction(step)
	case containerActionRun:
		return h.validateRunAction(step)
	case containerActionInspect:
		return h.validateInspectAction(step)
	default:
		return invalidContainerField(step, "action", action, "Action must be `build`, `push`, `run`, `inspect`, or empty for `run`")
	}
}

// Execute runs the container step.
func (h *ContainerHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ContainerHandler.Execute")()

	return h.ExecuteWithWorkflow(ctx, step, vars, nil)
}

// ExecuteWithWorkflow runs the container step with workflow output inheritance.
func (h *ContainerHandler) ExecuteWithWorkflow(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	defer perf.Track(nil, "step.ContainerHandler.ExecuteWithWorkflow")()

	switch containerStepAction(step) {
	case containerActionBuild:
		return h.executeBuild(ctx, step, vars)
	case containerActionPush:
		return h.executePush(ctx, step, vars)
	case containerActionInspect:
		return h.executeInspect(ctx, step, vars)
	default:
		return h.executeRun(ctx, step, vars, workflow)
	}
}

// applyRuntimeEnv forwards the step's resolved environment to the container
// runtime so its CLI subprocesses (build/push/run) can use credentials
// materialized by auth integrations — e.g. the DOCKER_CONFIG written by the
// aws/ecr integration after the workflow executor authenticates the step
// `identity:`. Runtimes that don't support a custom environment are left to
// inherit os.Environ().
func applyRuntimeEnv(runtime container.Runtime, vars *Variables) {
	if setter, ok := runtime.(container.EnvSetter); ok {
		setter.SetEnv(vars.EnvSlice())
	}
}

func resolveOptional(vars *Variables, value, field, stepName string) (string, error) {
	if value == "" {
		return "", nil
	}
	resolved, err := vars.Resolve(value)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve %s: %w", stepName, field, err)
	}
	return resolved, nil
}

func resolveStringSlice(vars *Variables, values []string, field, stepName string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	resolved := make([]string, 0, len(values))
	for _, value := range values {
		item, err := resolveOptional(vars, value, field, stepName)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, item)
	}
	return resolved, nil
}

func containerStepAction(step *schema.WorkflowStep) string {
	action := strings.TrimSpace(step.Action)
	if action == "" {
		return containerActionRun
	}
	return action
}

func effectiveBuildStep(step *schema.WorkflowStep) schema.ContainerBuildStep {
	if step.Build == nil {
		return schema.ContainerBuildStep{}
	}
	return *step.Build
}

func effectivePushStep(step *schema.WorkflowStep) schema.ContainerPushStep {
	if step.Push == nil {
		return schema.ContainerPushStep{}
	}
	return *step.Push
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (h *ContainerHandler) writeOutput(step *schema.WorkflowStep, workflow *schema.WorkflowDefinition, stdout, stderr string) {
	mode := GetOutputMode(step, workflow)
	if mode == OutputModeNone {
		return
	}
	if stdout != "" {
		_ = data.Write(stdout)
	}
	if stderr != "" {
		ui.Write(stderr)
	}
}

func isValidContainerRuntime(value string) bool {
	return value == "" || value == string(container.TypeDocker) || value == string(container.TypePodman)
}

func isValidContainerBuildEngine(value string) bool {
	return value == "" || value == containerBuildEngineBuildx
}

func isValidContainerPull(value string) bool {
	return value == "" || value == container.PullMissing || value == container.PullAlways || value == container.PullNever
}

func isValidContainerCleanup(value string) bool {
	return value == "" || value == container.CleanupAlways || value == container.CleanupOnSuccess || value == container.CleanupNever
}

func invalidContainerField(step *schema.WorkflowStep, field, value, explanation string) error {
	return errUtils.Build(errUtils.ErrStepFieldRequired).
		WithContext("step", step.Name).
		WithContext("field", field).
		WithContext("value", value).
		WithExplanation(explanation).
		Err()
}

var containerNamePattern = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func containerStepName(stepName string) string {
	if stepName == "" {
		stepName = "step"
	}
	safeName := strings.Trim(containerNamePattern.ReplaceAllString(stepName, "-"), "-.")
	if safeName == "" {
		safeName = "step"
	}
	return fmt.Sprintf("atmos-step-%s-%d", safeName, time.Now().UnixNano())
}
