package step

import (
	"context"
	"fmt"
	"maps"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	// Register supported file publishers independently of component imports.
	_ "github.com/cloudposse/atmos/pkg/provisioner/target/git"
	_ "github.com/cloudposse/atmos/pkg/provisioner/target/s3"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PublishHandler publishes local files to a named or inline delivery target.
type PublishHandler struct{ BaseHandler }

func init() {
	Register(&PublishHandler{BaseHandler: NewBaseHandler("publish", CategoryCommand, false)})
}

// Validate checks the fields required for publishing.
func (h *PublishHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.PublishHandler.Validate")()
	source, ok := step.Source.(string)
	if !ok || strings.TrimSpace(source) == "" {
		return errUtils.ErrPublishSource
	}
	if step.Action != "" {
		return fmt.Errorf("%w: publish has no action field", errUtils.ErrPublishTarget)
	}
	switch t := step.Target.(type) {
	case string:
		if strings.TrimSpace(t) != "" {
			return nil
		}
	case map[string]any:
		if len(t) > 0 {
			return nil
		}
	}
	return fmt.Errorf("%w: target must be a name or mapping", errUtils.ErrPublishTarget)
}

// Execute validates local files before authenticating and publishing them.
func (h *PublishHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(vars.AtmosConfig, "step.PublishHandler.Execute")()
	if err := h.Validate(step); err != nil {
		return nil, err
	}
	name, block, err := resolvePublishTarget(step.Target, vars)
	if err != nil {
		return nil, err
	}
	kind, _ := block["kind"].(string)
	publisher, err := target.Publisher(kind)
	if err != nil {
		return nil, err
	}
	in, err := h.publishInput(step, vars, block)
	if err != nil {
		return nil, err
	}
	in.Metadata.Target = name
	if err := publisher.ValidatePublish(in); err != nil {
		return nil, err
	}
	options := publishAuthOptions(step, vars, name, block)
	if err := auth.ValidateTargetAuth(options); err != nil {
		return nil, err
	}
	if step.DryRun {
		return NewStepResult(name).WithSkipped(), nil
	}
	if err := authenticatePublish(in, options); err != nil {
		return nil, err
	}
	result, err := publisher.Publish(ctx, in)
	if err != nil {
		return nil, err
	}
	out := publishStepResult(name, kind, result)
	reportPublishResult(ctx, step, vars, out)
	return out, nil
}

func (h *PublishHandler) publishInput(step *schema.WorkflowStep, vars *Variables, block map[string]any) (*target.PublishInput, error) {
	source, err := h.ResolveInWorkingDirectory(step, vars, step.Source.(string), "source")
	if err != nil {
		return nil, err
	}
	dest, err := vars.Resolve(step.Destination)
	if err != nil {
		return nil, err
	}
	files, err := target.LocalPublishFiles(source, dest)
	if err != nil {
		return nil, err
	}
	config := vars.AtmosConfig
	if config == nil {
		config = &schema.AtmosConfiguration{}
	}
	env, err := publishEnvironment(step, vars)
	if err != nil {
		return nil, err
	}
	in := &target.PublishInput{AtmosConfig: config, TargetConfig: block, Files: files, Env: env}
	if vars.PublishInfo != nil {
		in.Metadata = target.ArtifactMetadata{Component: vars.PublishInfo.ComponentFromArg, Stack: vars.PublishInfo.Stack}
	}
	return in, nil
}

func publishEnvironment(step *schema.WorkflowStep, vars *Variables) (map[string]string, error) {
	env := maps.Clone(vars.Env)
	if env == nil {
		env = make(map[string]string)
	}
	overlay, err := vars.ResolveEnvMap(step.Env)
	if err != nil {
		return nil, err
	}
	maps.Copy(env, overlay)
	return env, nil
}

func resolvePublishTarget(value any, vars *Variables) (string, map[string]any, error) {
	name := "inline"
	block, ok := value.(map[string]any)
	if !ok {
		var err error
		name, err = vars.Resolve(value.(string))
		if err != nil {
			return "", nil, err
		}
		if vars.PublishInfo == nil {
			return "", nil, fmt.Errorf("%w: named target %q requires component scope; use an inline target", errUtils.ErrPublishTarget, name)
		}
		provision, _ := vars.PublishInfo.ComponentSection["provision"].(map[string]any)
		targets, _ := provision["targets"].(map[string]any)
		block, ok = targets[name].(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("%w: target %q not found", errUtils.ErrPublishTarget, name)
		}
	}
	resolved, err := resolvePublishMap(block, vars)
	return name, resolved, err
}

func resolvePublishMap(block map[string]any, vars *Variables) (map[string]any, error) {
	resolved := maps.Clone(block)
	for key, value := range block {
		var err error
		switch v := value.(type) {
		case string:
			resolved[key], err = vars.Resolve(v)
		case map[string]any:
			resolved[key], err = resolvePublishMap(v, vars)
		}
		if err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

func publishStepResult(name, kind string, result *target.PublishResult) *StepResult {
	value := name
	if len(result.Locations) == 1 {
		value = result.Locations[0]
	}
	out := NewStepResult(value).WithValues(result.Locations).
		WithMetadata("target", name).WithMetadata("kind", kind).
		WithMetadata("changed", result.Changed).WithMetadata("unchanged", result.Unchanged)
	for key, value := range result.Metadata {
		out.WithMetadata(key, value)
	}
	return out
}

// reportPublishResult shows the actual artifact locations on the scoped UI
// stream. Locations include unchanged files; the counts distinguish writes
// from skipped uploads without claiming every listed file changed.
func reportPublishResult(ctx context.Context, step *schema.WorkflowStep, vars *Variables, result *StepResult) {
	if OutputSuppressed(ctx) || step.Output == string(OutputModeNone) {
		return
	}
	verb := "written"
	if result.Metadata["kind"] == "aws/s3" {
		verb = "uploaded"
	}
	vars.UI().Infof("Publish %s (%s): %d %s, %d unchanged", result.Metadata["target"], result.Metadata["kind"], result.Metadata["changed"], verb, result.Metadata["unchanged"])
	if result.Metadata["kind"] == "git" {
		vars.UI().Infof("Repository: %s (branch %s)", result.Metadata["repository"], result.Metadata["branch"])
	}
	for _, location := range result.Values {
		vars.UI().Writef("  → %s\n", location)
	}
}
