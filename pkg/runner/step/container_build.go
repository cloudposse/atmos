package step

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// validateBuildAction checks the configuration of a `build` container step.
func validateBuildAction(step *schema.WorkflowStep) error {
	build := effectiveBuildStep(step)
	if !isValidContainerRuntime(build.Provider) {
		return invalidContainerField(step, "build.provider", build.Provider, "Provider must be `docker`, `podman`, or empty for auto-detect")
	}
	if !isValidContainerBuildEngine(build.Engine) {
		return invalidContainerField(step, "build.engine", build.Engine, "Build engine must be `buildx` or empty for the runtime default")
	}
	if (build.Engine == containerBuildEngineBuildx || build.Bake != nil) && build.Provider != string(container.TypeDocker) {
		return invalidContainerField(step, "build.provider", build.Provider, "Docker Buildx and Bake require `provider: docker` in V1; Podman uses the native `podman build` path")
	}
	return nil
}

func (h *ContainerHandler) executeBuild(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	build := effectiveBuildStep(step)
	buildConfig, err := h.buildBuildConfig(step, vars)
	if err != nil {
		return nil, err
	}

	runtimeName := strings.TrimSpace(build.Provider)
	if step.DryRun {
		preview := container.BuildImageBuildPreview(runtimeName, buildConfig)
		ui.Writeln(preview)
		return NewStepResult(firstString(buildConfig.Tags)).
			WithMetadata(exitCodeMetadata, 0).
			WithMetadata("image", firstString(buildConfig.Tags)), nil
	}

	runtime, err := container.DetectRuntimeWithPreferenceAndRecovery(ctx, runtimeName, build.RuntimeAutoStart)
	if err != nil {
		return nil, err
	}
	applyRuntimeEnv(runtime, vars)

	image := firstString(buildConfig.Tags)
	// Show a spinner while the runtime builds the image (it streams nothing on
	// success), mirroring the devcontainer build UX. Degrades to a ✓ line off-TTY.
	buildErr := spinner.ExecWithSpinner(
		buildSpinnerMessage("Building image", image),
		buildSpinnerMessage("Built image", image),
		func() error { return runtime.Build(ctx, buildConfig) },
	)
	if buildErr != nil {
		return NewStepResult(image).
			WithMetadata(exitCodeMetadata, 1).
			WithMetadata("image", image).
			WithError(buildErr.Error()), buildErr
	}

	stepResult := NewStepResult(image).
		WithMetadata("image", image).
		WithMetadata(exitCodeMetadata, 0)
	if image != "" {
		if info, inspectErr := runtime.ImageInspect(ctx, image); inspectErr == nil && info != nil {
			stepResult.WithMetadata("image_id", info.ID).
				WithMetadata("repo_tags", info.RepoTags).
				WithMetadata("repo_digests", info.RepoDigests)
			writeContainerImageSummary(vars.AtmosConfig, info, container.ImageSummaryOptions{Image: image})
		} else if inspectErr != nil {
			log.Debug("container step: failed to inspect built image for CI summary", "image", image, "error", inspectErr)
		}
	}
	return stepResult, nil
}

// buildSpinnerMessage renders a spinner status line, omitting the image when a
// build (e.g. Bake without tags) has no resolvable image reference.
func buildSpinnerMessage(verb, image string) string {
	if image == "" {
		return verb
	}
	return fmt.Sprintf("%s %s", verb, image)
}

func (h *ContainerHandler) buildBuildConfig(step *schema.WorkflowStep, vars *Variables) (*container.BuildConfig, error) {
	build := effectiveBuildStep(step)
	contextDir, err := resolveOptional(vars, defaultString(build.Context, "."), "build.context", step.Name)
	if err != nil {
		return nil, err
	}
	dockerfile, err := resolveOptional(vars, defaultString(build.Dockerfile, "Dockerfile"), "build.dockerfile", step.Name)
	if err != nil {
		return nil, err
	}
	target, err := resolveOptional(vars, build.Target, "build.target", step.Name)
	if err != nil {
		return nil, err
	}
	tags, err := resolveStringSlice(vars, build.Tags, "build.tags", step.Name)
	if err != nil {
		return nil, err
	}
	buildArgs, err := vars.ResolveEnvMap(build.BuildArgs)
	if err != nil {
		return nil, err
	}
	bake, err := resolveBuildBake(vars, build.Bake, step.Name)
	if err != nil {
		return nil, err
	}
	return &container.BuildConfig{
		Dockerfile: dockerfile,
		Context:    contextDir,
		Engine:     strings.TrimSpace(build.Engine),
		Args:       buildArgs,
		Tags:       tags,
		Target:     target,
		NoCache:    build.NoCache,
		Pull:       build.Pull,
		Bake:       bake,
	}, nil
}

func resolveBuildBake(vars *Variables, bake *schema.ContainerBuildBakeStep, stepName string) (*container.BakeConfig, error) {
	if bake == nil {
		return nil, nil
	}
	file, err := resolveOptional(vars, bake.File, "build.bake.file", stepName)
	if err != nil {
		return nil, err
	}
	files, err := resolveStringSlice(vars, bake.Files, "build.bake.files", stepName)
	if err != nil {
		return nil, err
	}
	target, err := resolveOptional(vars, bake.Target, "build.bake.target", stepName)
	if err != nil {
		return nil, err
	}
	targets, err := resolveStringSlice(vars, bake.Targets, "build.bake.targets", stepName)
	if err != nil {
		return nil, err
	}
	set, err := resolveStringSlice(vars, bake.Set, "build.bake.set", stepName)
	if err != nil {
		return nil, err
	}
	varsMap, err := vars.ResolveEnvMap(bake.Vars)
	if err != nil {
		return nil, err
	}
	return &container.BakeConfig{
		File:    file,
		Files:   files,
		Target:  target,
		Targets: targets,
		Set:     set,
		Vars:    varsMap,
		Load:    bake.Load,
		Push:    bake.Push,
		Print:   bake.Print,
	}, nil
}
