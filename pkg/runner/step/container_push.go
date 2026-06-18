package step

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// validatePushAction checks the configuration of a `push` container step.
func (h *ContainerHandler) validatePushAction(step *schema.WorkflowStep) error {
	push := effectivePushStep(step)
	if err := h.ValidateRequired(step, "push.image", push.Image); err != nil {
		return err
	}
	if !isValidContainerRuntime(push.Runtime) {
		return invalidContainerField(step, "push.runtime", push.Runtime, "Runtime must be `docker`, `podman`, or empty for auto-detect")
	}
	return nil
}

func (h *ContainerHandler) executePush(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	push := effectivePushStep(step)
	pushConfig, tags, err := h.buildPushConfig(step, vars)
	if err != nil {
		return nil, err
	}

	runtimeName := strings.TrimSpace(push.Runtime)
	if step.DryRun {
		return previewPush(runtimeName, pushConfig, tags), nil
	}

	runtime, err := container.DetectRuntimeWithPreferenceAndRecovery(ctx, runtimeName, push.RuntimeAutoStart)
	if err != nil {
		return nil, err
	}

	return runPushImages(ctx, runtime, pushConfig, tags)
}

// previewPush renders the dry-run output for a push step.
func previewPush(runtimeName string, pushConfig *resolvedPushConfig, tags []string) *StepResult {
	previews := make([]string, 0, len(tags)*2+1)
	for _, tag := range tags {
		previews = append(previews, container.BuildImageTagPreview(runtimeName, pushConfig.Image, tag))
		previews = append(previews, container.BuildImagePushPreview(runtimeName, tag))
	}
	if len(tags) == 0 {
		previews = append(previews, container.BuildImagePushPreview(runtimeName, pushConfig.Image))
	}
	preview := strings.Join(previews, "\n")
	ui.Writeln(preview)
	return NewStepResult(pushConfig.Image).
		WithMetadata(exitCodeMetadata, 0).
		WithMetadata("image", pushConfig.Image)
}

// runPushImages tags (when needed) and pushes each resolved image tag.
func runPushImages(ctx context.Context, runtime container.Runtime, pushConfig *resolvedPushConfig, tags []string) (*StepResult, error) {
	images := tags
	if len(images) == 0 {
		images = []string{pushConfig.Image}
	}

	var last *container.PushResult
	outputs := make([]string, 0, len(images))
	for _, image := range images {
		if image != pushConfig.Image {
			if err := runtime.Tag(ctx, pushConfig.Image, image); err != nil {
				return NewStepResult(image).WithMetadata(exitCodeMetadata, 1).WithError(err.Error()), err
			}
		}
		pushed, err := runtime.Push(ctx, image)
		if pushed != nil {
			last = pushed
			outputs = append(outputs, pushed.Output)
		}
		if err != nil {
			return NewStepResult(image).
				WithMetadata("image", image).
				WithMetadata("digest", metadataString(pushed, "digest")).
				WithMetadata("stdout", strings.Join(outputs, "\n")).
				WithMetadata(exitCodeMetadata, 1).
				WithError(err.Error()), err
		}
	}

	image := pushConfig.Image
	digest := ""
	if last != nil {
		image = last.Image
		digest = last.Digest
	}

	return NewStepResult(image).
		WithMetadata("image", image).
		WithMetadata("digest", digest).
		WithMetadata("stdout", strings.Join(outputs, "\n")).
		WithMetadata("stderr", "").
		WithMetadata(exitCodeMetadata, 0), nil
}

type resolvedPushConfig struct {
	Image string
}

func (h *ContainerHandler) buildPushConfig(step *schema.WorkflowStep, vars *Variables) (*resolvedPushConfig, []string, error) {
	push := effectivePushStep(step)
	image, err := resolveOptional(vars, push.Image, "push.image", step.Name)
	if err != nil {
		return nil, nil, err
	}
	tags, err := resolveStringSlice(vars, push.Tags, "push.tags", step.Name)
	if err != nil {
		return nil, nil, err
	}
	return &resolvedPushConfig{Image: image}, tags, nil
}

func metadataString(result *container.PushResult, key string) string {
	if result == nil {
		return ""
	}
	switch key {
	case "digest":
		return result.Digest
	case "image":
		return result.Image
	default:
		return ""
	}
}
