package step

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	sourceprov "github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	ErrWorkdirPathRequired     = errUtils.ErrWorkdirPathRequired
	ErrWorkdirSourceRequired   = errUtils.ErrWorkdirSourceRequired
	ErrWorkdirSourceKeyInvalid = errUtils.ErrWorkdirSourceKeyInvalid
)

// WorkdirHandler provisions a mutable working directory from a source.
type WorkdirHandler struct {
	BaseHandler
}

func init() {
	Register(&WorkdirHandler{
		BaseHandler: NewBaseHandler(schema.TaskTypeWorkdir, CategoryCommand, false),
	})
}

// Validate checks that the step has required fields.
func (h *WorkdirHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.WorkdirHandler.Validate")()

	if err := h.ValidateRequired(step, "path", step.Path); err != nil {
		return err
	}
	if step.Source == nil {
		return h.ValidateRequired(step, "source", "")
	}
	return nil
}

// Execute provisions the configured source into the target path.
func (h *WorkdirHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.WorkdirHandler.Execute")()

	targetPath, err := vars.Resolve(step.Path)
	if err != nil {
		return nil, fmt.Errorf("step '%s': failed to resolve path: %w", step.Name, err)
	}
	if targetPath == "" {
		return nil, errUtils.Build(ErrWorkdirPathRequired).
			WithContext("step", step.Name).
			Err()
	}

	sourceSpec, err := h.resolveSourceSpec(step, vars)
	if err != nil {
		return nil, err
	}

	if err := sourceprov.VendorSource(ctx, nil, sourceSpec, targetPath, sourceprov.WithReplaceTarget(step.Reset)); err != nil {
		if !step.Reset {
			return nil, fmt.Errorf("step '%s': failed to provision source %q to %q; set reset: true to replace an existing target: %w", step.Name, sourceSpec.Uri, targetPath, err)
		}
		return nil, fmt.Errorf("step '%s': failed to provision source %q to %q: %w", step.Name, sourceSpec.Uri, targetPath, err)
	}

	return NewStepResult(targetPath).
		WithMetadata("path", targetPath).
		WithMetadata("source", sourceSpec.Uri), nil
}

func (h *WorkdirHandler) resolveSourceSpec(step *schema.WorkflowStep, vars *Variables) (*schema.VendorComponentSource, error) {
	resolved, err := resolveWorkdirSourceValue(step.Source, vars)
	if err != nil {
		return nil, fmt.Errorf("step '%s': failed to resolve source: %w", step.Name, err)
	}
	spec, err := sourceprov.ExtractSource(map[string]any{"source": resolved})
	if err != nil {
		return nil, fmt.Errorf("step '%s': invalid source: %w", step.Name, err)
	}
	if spec == nil || spec.Uri == "" {
		return nil, errUtils.Build(ErrWorkdirSourceRequired).
			WithContext("step", step.Name).
			Err()
	}
	return spec, nil
}

func resolveWorkdirSourceValue(value any, vars *Variables) (any, error) {
	switch v := value.(type) {
	case string:
		return vars.Resolve(v)
	case map[string]any:
		return resolveStringSourceMap(v, vars)
	case map[any]any:
		return resolveAnySourceMap(v, vars)
	case []any:
		return resolveSourceSlice(v, vars)
	default:
		return value, nil
	}
}

func resolveStringSourceMap(input map[string]any, vars *Variables) (map[string]any, error) {
	out := make(map[string]any, len(input))
	for key, val := range input {
		resolved, err := resolveWorkdirSourceValue(val, vars)
		if err != nil {
			return nil, err
		}
		out[key] = resolved
	}
	return out, nil
}

func resolveAnySourceMap(input map[any]any, vars *Variables) (map[string]any, error) {
	out := make(map[string]any, len(input))
	for key, val := range input {
		keyString, ok := key.(string)
		if !ok {
			return nil, errUtils.Build(ErrWorkdirSourceKeyInvalid).Err()
		}
		resolved, err := resolveWorkdirSourceValue(val, vars)
		if err != nil {
			return nil, err
		}
		out[keyString] = resolved
	}
	return out, nil
}

func resolveSourceSlice(input []any, vars *Variables) ([]any, error) {
	out := make([]any, len(input))
	for i, val := range input {
		resolved, err := resolveWorkdirSourceValue(val, vars)
		if err != nil {
			return nil, err
		}
		out[i] = resolved
	}
	return out, nil
}
