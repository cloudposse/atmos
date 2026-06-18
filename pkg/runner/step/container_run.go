package step

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// validateRunAction checks the configuration of a `run` container step.
func (h *ContainerHandler) validateRunAction(step *schema.WorkflowStep) error {
	run := effectiveRunStep(step)
	if err := h.ValidateRequired(step, "run.image", run.Image); err != nil {
		return err
	}
	if err := h.ValidateRequired(step, "run.command", run.Command); err != nil {
		return err
	}
	if !isValidContainerRuntime(run.Runtime) {
		return invalidContainerField(step, "run.runtime", run.Runtime, "Runtime must be `docker`, `podman`, or empty for auto-detect")
	}
	if !isValidContainerPull(run.Pull) {
		return invalidContainerField(step, "run.pull", run.Pull, "Pull policy must be `missing`, `always`, `never`, or empty")
	}
	if !isValidContainerCleanup(run.Cleanup) {
		return invalidContainerField(step, "run.cleanup", run.Cleanup, "Cleanup policy must be `always`, `on_success`, `never`, or empty")
	}
	return nil
}

func (h *ContainerHandler) executeRun(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	config, run, err := h.buildRunConfig(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	runtimeName := strings.TrimSpace(run.Runtime)
	if step.DryRun {
		preview := container.BuildEphemeralPreview(runtimeName, config)
		ui.Writeln(preview)
		return NewStepResult(preview).WithMetadata(exitCodeMetadata, 0), nil
	}

	runtime, err := container.DetectRuntimeWithPreferenceAndRecovery(ctx, runtimeName, run.RuntimeAutoStart)
	if err != nil {
		return nil, err
	}
	applyRuntimeEnv(runtime, vars)

	result, err := container.RunEphemeralContainer(ctx, runtime, config)
	if result == nil {
		return nil, err
	}

	if !step.Tty && !step.Interactive {
		h.writeOutput(step, workflow, result.Stdout, result.Stderr)
	}

	stepResult := NewStepResult(result.Stdout).
		WithMetadata("stdout", result.Stdout).
		WithMetadata("stderr", result.Stderr).
		WithMetadata(exitCodeMetadata, result.ExitCode).
		WithMetadata("container_id", result.ContainerID)
	if err != nil {
		stepResult.WithError(result.Stderr)
	}
	return stepResult, err
}

// resolveRunCommand resolves the command for a run step, falling back to the
// generic command resolver when no explicit run command is configured.
func (h *ContainerHandler) resolveRunCommand(ctx context.Context, step *schema.WorkflowStep, vars *Variables, run *schema.ContainerRunStep) (string, error) {
	if run.Command != "" {
		resolved, err := vars.Resolve(run.Command)
		if err != nil {
			return "", fmt.Errorf("step '%s': failed to resolve command: %w", step.Name, err)
		}
		return resolved, nil
	}
	return h.ResolveCommand(ctx, step, vars)
}

// resolvedRunBasics holds the resolved scalar fields shared by a run step.
type resolvedRunBasics struct {
	image     string
	shell     string
	workspace string
}

// resolveRunBasics resolves and defaults the image, shell, and workspace fields.
func resolveRunBasics(vars *Variables, run *schema.ContainerRunStep, stepName string) (resolvedRunBasics, error) {
	image, err := resolveOptional(vars, run.Image, "run.image", stepName)
	if err != nil {
		return resolvedRunBasics{}, err
	}
	shell, err := resolveOptional(vars, run.Shell, "run.shell", stepName)
	if err != nil {
		return resolvedRunBasics{}, err
	}
	if shell == "" {
		shell = defaultContainerShell
	}
	workspace, err := resolveOptional(vars, run.Workspace, "run.workspace", stepName)
	if err != nil {
		return resolvedRunBasics{}, err
	}
	if workspace == "" {
		workspace = defaultContainerWorkdir
	}
	return resolvedRunBasics{image: image, shell: shell, workspace: workspace}, nil
}

func (h *ContainerHandler) buildRunConfig(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*container.EphemeralConfig, schema.ContainerRunStep, error) {
	run := effectiveRunStep(step)
	command, err := h.resolveRunCommand(ctx, step, vars, &run)
	if err != nil {
		return nil, run, err
	}

	basics, err := resolveRunBasics(vars, &run, step.Name)
	if err != nil {
		return nil, run, err
	}

	workDir, err := resolveWorkDir(vars, step)
	if err != nil {
		return nil, run, err
	}

	env, err := resolveContainerEnv(vars, step)
	if err != nil {
		return nil, run, err
	}

	mounts, err := convertContainerMounts(vars, run.Mounts)
	if err != nil {
		return nil, run, err
	}
	mounts = append(mounts, credentialFileMounts(env)...)

	ports := convertContainerPorts(run.Ports)

	return &container.EphemeralConfig{
		Name:              containerStepName(step.Name),
		Image:             basics.image,
		Command:           []string{basics.shell, "-lc", command},
		WorkspaceHostPath: workDir,
		WorkspaceFolder:   basics.workspace,
		WorkspaceReadOnly: run.WorkspaceReadOnly,
		Mounts:            mounts,
		Ports:             ports,
		Env:               env,
		User:              run.User,
		RunArgs:           run.RunArgs,
		PullPolicy:        run.Pull,
		CleanupPolicy:     run.Cleanup,
		TTY:               step.Tty,
		Interactive:       step.Interactive,
		Labels: map[string]string{
			"com.atmos.step.type": containerStepType,
			"com.atmos.step.name": step.Name,
		},
	}, run, nil
}

func (h *ContainerHandler) buildConfig(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*container.EphemeralConfig, error) {
	config, _, err := h.buildRunConfig(ctx, step, vars)
	return config, err
}

func effectiveRunStep(step *schema.WorkflowStep) schema.ContainerRunStep {
	run := schema.ContainerRunStep{}
	if step.Run != nil {
		run = *step.Run
	}
	mergeRunScalarFields(&run, step)
	mergeRunSliceFields(&run, step)
	run.RuntimeAutoStart = run.RuntimeAutoStart || step.RuntimeAutoStart
	run.WorkspaceReadOnly = run.WorkspaceReadOnly || step.WorkspaceReadOnly
	return run
}

// mergeRunScalarFields fills empty scalar run fields from the step-level shorthand.
func mergeRunScalarFields(run *schema.ContainerRunStep, step *schema.WorkflowStep) {
	if run.Image == "" {
		run.Image = step.Image
	}
	if run.Command == "" {
		run.Command = step.Command
	}
	if run.Shell == "" {
		run.Shell = step.Shell
	}
	if run.Runtime == "" {
		run.Runtime = step.Runtime
	}
	if run.Pull == "" {
		run.Pull = step.Pull
	}
	if run.Workspace == "" {
		run.Workspace = step.Workspace
	}
	if run.Cleanup == "" {
		run.Cleanup = step.Cleanup
	}
	if run.User == "" {
		run.User = step.User
	}
}

// mergeRunSliceFields fills empty slice run fields from the step-level shorthand.
func mergeRunSliceFields(run *schema.ContainerRunStep, step *schema.WorkflowStep) {
	if len(run.RunArgs) == 0 {
		run.RunArgs = step.RunArgs
	}
	if len(run.Mounts) == 0 {
		run.Mounts = step.Mounts
	}
	if len(run.Ports) == 0 {
		run.Ports = step.Ports
	}
}

func resolveWorkDir(vars *Variables, step *schema.WorkflowStep) (string, error) {
	workDir := step.WorkingDirectory
	if workDir != "" {
		resolved, err := vars.Resolve(workDir)
		if err != nil {
			return "", fmt.Errorf("step '%s': failed to resolve working_directory: %w", step.Name, err)
		}
		workDir = resolved
	}
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	if !filepath.IsAbs(workDir) {
		abs, err := filepath.Abs(workDir)
		if err != nil {
			return "", err
		}
		workDir = abs
	}
	return workDir, nil
}

func resolveContainerEnv(vars *Variables, step *schema.WorkflowStep) ([]string, error) {
	if len(step.Env) == 0 {
		return nil, nil
	}
	resolvedEnv, err := vars.ResolveEnvMap(step.Env)
	if err != nil {
		return nil, fmt.Errorf("step '%s': %w", step.Name, err)
	}
	env := make([]string, 0, len(resolvedEnv))
	for k, v := range resolvedEnv {
		env = append(env, k+"="+v)
	}
	return env, nil
}

func convertContainerMounts(vars *Variables, mounts []schema.ContainerMount) ([]container.Mount, error) {
	result := make([]container.Mount, 0, len(mounts))
	for _, mount := range mounts {
		source, err := resolveOptional(vars, mount.Source, "mount source", "container")
		if err != nil {
			return nil, err
		}
		target, err := resolveOptional(vars, mount.Target, "mount target", "container")
		if err != nil {
			return nil, err
		}
		result = append(result, container.Mount{
			Type:     defaultMountType(mount.Type),
			Source:   expandHostPath(source),
			Target:   target,
			ReadOnly: mount.ReadOnly,
		})
	}
	return result, nil
}

func convertContainerPorts(ports []schema.ContainerPort) []container.PortBinding {
	result := make([]container.PortBinding, 0, len(ports))
	for _, port := range ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		result = append(result, container.PortBinding{
			HostPort:      port.Host,
			ContainerPort: port.Container,
			Protocol:      protocol,
		})
	}
	return result
}

func defaultMountType(mountType string) string {
	if mountType == "" {
		return "bind"
	}
	return mountType
}

func expandHostPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := homedir.Dir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return os.ExpandEnv(path)
}

func credentialFileMounts(env []string) []container.Mount {
	mounts := make([]container.Mount, 0)
	seen := make(map[string]struct{})
	for _, entry := range env {
		_, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" || !filepath.IsAbs(value) {
			continue
		}
		info, err := os.Stat(value)
		if err != nil || info.IsDir() {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		mounts = append(mounts, container.Mount{
			Type:     "bind",
			Source:   value,
			Target:   value,
			ReadOnly: true,
		})
	}
	return mounts
}
