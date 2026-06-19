package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	workflowContainerDefaultShell = "/bin/sh"
	// Relative path token referring to the current working directory.
	currentDir = "."
)

// ContainerStepParams carries workflow and step execution inputs for the workflow
// container entry points. It keeps the public functions within the
// argument limit and groups related values together.
type ContainerStepParams struct {
	Workflow     string
	WorkflowPath string
	BasePath     string
	WorkflowDef  *schema.WorkflowDefinition
	Step         *schema.WorkflowStep
	HostWorkDir  string
	Command      string
	StepEnv      []string
	RuntimeEnv   []string
	DryRun       bool
}

// ContainerSession owns the long-lived container backing a workflow run.
type ContainerSession struct {
	backend       containerBackend
	config        *schema.WorkflowContainer
	hostWorkspace string
}

type containerBackend interface {
	ID() string
	Name() string
	Exec(ctx context.Context, command []string, opts *container.ExecOptions) error
	Cleanup(success bool) error
}

// CalculateWorkingDirectory determines the working directory for a workflow step.
// Step-level working_directory overrides workflow-level. Relative paths resolve
// against basePath.
func CalculateWorkingDirectory(workflowDef *schema.WorkflowDefinition, step *schema.WorkflowStep, basePath string) string {
	workDir := strings.TrimSpace(workflowDef.WorkingDirectory)
	if stepWorkDir := strings.TrimSpace(step.WorkingDirectory); stepWorkDir != "" {
		workDir = stepWorkDir
	}
	if workDir == "" {
		return ""
	}
	if !filepath.IsAbs(workDir) {
		resolvedBasePath := basePath
		if strings.TrimSpace(resolvedBasePath) == "" {
			resolvedBasePath = currentDir
		}
		workDir = filepath.Join(resolvedBasePath, workDir)
	}
	return workDir
}

// StepContainerDisabled reports whether the step explicitly opts out of the workflow container.
func StepContainerDisabled(step *schema.WorkflowStep) bool {
	return step.Container != nil && !step.Container.IsEnabled()
}

// StepContainerOverride reports whether the step has an enabled container override.
func StepContainerOverride(step *schema.WorkflowStep) bool {
	return step.Container != nil && step.Container.IsEnabled()
}

// StartWorkflowContainer starts the workflow-level container if configured.
func StartWorkflowContainer(ctx context.Context, params *ContainerStepParams) (*ContainerSession, error) {
	defer perf.Track(nil, "workflow.StartWorkflowContainer")()

	if params == nil {
		return nil, errUtils.ErrNilParam
	}
	workflowDef := params.WorkflowDef
	if workflowDef == nil || workflowDef.Container == nil || !workflowDef.Container.IsEnabled() {
		return nil, nil
	}

	hostWorkspace, err := workflowContainerHostWorkspace(workflowDef, params.BasePath)
	if err != nil {
		return nil, err
	}
	cfg, err := buildContainerConfig(params, workflowDef.Container, hostWorkspace)
	if err != nil {
		return nil, err
	}
	backend, err := container.StartSandbox(ctx, &cfg)
	if err != nil {
		return nil, err
	}
	if params.DryRun {
		ui.Writef("%s create %s %s\n", runtimePreviewName(cfg.RuntimeName), cfg.Name, cfg.Image)
	}
	return &ContainerSession{backend: backend, config: workflowDef.Container, hostWorkspace: hostWorkspace}, nil
}

func workflowContainerHostWorkspace(workflowDef *schema.WorkflowDefinition, basePath string) (string, error) {
	workDir := strings.TrimSpace(workflowDef.WorkingDirectory)
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	} else if !filepath.IsAbs(workDir) {
		if strings.TrimSpace(basePath) == "" {
			basePath = currentDir
		}
		workDir = filepath.Join(basePath, workDir)
	}
	return filepath.Abs(workDir)
}

func buildContainerConfig(params *ContainerStepParams, cfg *schema.WorkflowContainer, hostWorkspace string) (container.SandboxConfig, error) {
	if strings.TrimSpace(cfg.Image) == "" {
		return container.SandboxConfig{}, fmt.Errorf("%w: workflow container image is required", errUtils.ErrContainerRuntimeOperation)
	}
	if !validRuntime(cfg.Provider) {
		return container.SandboxConfig{}, fmt.Errorf("%w: workflow container runtime must be docker, podman, or empty", errUtils.ErrContainerRuntimeOperation)
	}
	if !validPull(cfg.Pull) {
		return container.SandboxConfig{}, fmt.Errorf("%w: workflow container pull must be missing, always, never, or empty", errUtils.ErrContainerRuntimeOperation)
	}
	if !validCleanup(cfg.Cleanup) {
		return container.SandboxConfig{}, fmt.Errorf("%w: workflow container cleanup must be always, on_success, never, or empty", errUtils.ErrContainerRuntimeOperation)
	}

	containerCfg := container.NewWorkflowSandboxConfig(params.Workflow, params.WorkflowPath, hostWorkspace)
	containerCfg.RuntimeName = cfg.Provider
	containerCfg.RuntimeAutoStart = cfg.RuntimeAutoStart
	containerCfg.Image = cfg.Image
	containerCfg.WorkspaceFolder = defaultString(cfg.Workspace, "/workspace")
	containerCfg.WorkspaceReadOnly = cfg.WorkspaceReadOnly
	containerCfg.Mounts = convertWorkflowMounts(cfg.Mounts)
	containerCfg.Ports = convertWorkflowPorts(cfg.Ports)
	containerCfg.Env = envMapToSlice(cfg.Env)
	containerCfg.RuntimeEnv = params.RuntimeEnv
	containerCfg.User = cfg.User
	containerCfg.RunArgs = cfg.RunArgs
	containerCfg.PullPolicy = cfg.Pull
	containerCfg.CleanupPolicy = cfg.Cleanup
	containerCfg.DryRun = params.DryRun
	return containerCfg, nil
}

// ExecShell runs a shell command inside the workflow container.
func (s *ContainerSession) ExecShell(ctx context.Context, params *ContainerStepParams) error {
	defer perf.Track(nil, "workflow.ContainerSession.ExecShell")()

	if s == nil || s.backend == nil || params == nil || params.Step == nil || params.WorkflowDef == nil {
		return errUtils.ErrNilParam
	}
	step := params.Step
	containerWorkDir, err := mapHostWorkDirToContainer(params.HostWorkDir, s.hostWorkspace, defaultString(s.config.Workspace, "/workspace"))
	if err != nil {
		return err
	}
	shell := defaultString(s.config.Shell, workflowContainerDefaultShell)
	env := mergeEnvSlices(envMapToSlice(s.config.Env), params.StepEnv)
	cmd := []string{shell, "-lc", params.Command}
	if s.backend.ID() == s.backend.Name() {
		ui.Writef("%s exec %s %s\n", runtimePreviewName(s.config.Provider), s.backend.Name(), strings.Join(cmd, " "))
		return nil
	}

	if step.Tty || step.Interactive {
		return s.backend.Exec(ctx, cmd, &container.ExecOptions{
			User:         s.config.User,
			WorkingDir:   containerWorkDir,
			Env:          env,
			AttachStdin:  step.Interactive,
			AttachStdout: true,
			AttachStderr: true,
			Tty:          step.Tty,
			Stdin:        os.Stdin,
		})
	}

	writer := stepPkg.NewOutputModeWriter(stepPkg.GetOutputMode(step, params.WorkflowDef), step.Name, stepPkg.GetViewportConfig(step, params.WorkflowDef))
	_, _, err = writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		return s.backend.Exec(ctx, cmd, &container.ExecOptions{
			User:         s.config.User,
			WorkingDir:   containerWorkDir,
			Env:          env,
			AttachStdout: true,
			AttachStderr: true,
			Stdout:       stdout,
			Stderr:       stderr,
		})
	})
	return err
}

// RunStepContainerOverride runs a shell step in a one-shot step-level container.
func RunStepContainerOverride(ctx context.Context, params *ContainerStepParams) error {
	defer perf.Track(nil, "workflow.RunStepContainerOverride")()

	if params == nil || params.WorkflowDef == nil || params.Step == nil {
		return errUtils.ErrNilParam
	}
	workflowDef := params.WorkflowDef
	step := params.Step
	cfg := mergeWorkflowContainer(workflowDef.Container, step.Container)
	if !cfg.IsEnabled() {
		return nil
	}
	if strings.TrimSpace(cfg.Image) == "" {
		return fmt.Errorf("%w: step container image is required", errUtils.ErrContainerRuntimeOperation)
	}
	absHostWorkspace, err := resolveStepHostWorkspace(params)
	if err != nil {
		return err
	}
	ephemeral := buildEphemeralStepConfig(params, cfg, absHostWorkspace)
	if params.DryRun {
		ui.Writeln(container.BuildEphemeralPreview(runtimePreviewName(cfg.Provider), ephemeral))
		return nil
	}
	runtime, err := container.DetectRuntimeWithPreferenceAndRecovery(ctx, cfg.Provider, cfg.RuntimeAutoStart)
	if err != nil {
		return err
	}
	if setter, ok := runtime.(container.EnvSetter); ok {
		setter.SetEnv(params.RuntimeEnv)
	}
	result, err := container.RunEphemeralContainer(ctx, runtime, ephemeral)
	writeEphemeralResult(params, result, err)
	return err
}

// resolveStepHostWorkspace returns the absolute host workspace path for a step
// container override, defaulting to the workflow's host workspace when unset.
func resolveStepHostWorkspace(params *ContainerStepParams) (string, error) {
	hostWorkspace := params.HostWorkDir
	if hostWorkspace == "" {
		var err error
		hostWorkspace, err = workflowContainerHostWorkspace(params.WorkflowDef, params.BasePath)
		if err != nil {
			return "", err
		}
	}
	return filepath.Abs(hostWorkspace)
}

// buildEphemeralStepConfig assembles the one-shot container config for a step override.
func buildEphemeralStepConfig(params *ContainerStepParams, cfg *schema.WorkflowContainer, absHostWorkspace string) *container.EphemeralConfig {
	step := params.Step
	shell := defaultString(cfg.Shell, workflowContainerDefaultShell)
	return &container.EphemeralConfig{
		Name:              fmt.Sprintf("atmos-step-%s", step.Name),
		Image:             cfg.Image,
		Command:           []string{shell, "-lc", params.Command},
		WorkspaceHostPath: absHostWorkspace,
		WorkspaceFolder:   defaultString(cfg.Workspace, "/workspace"),
		WorkspaceReadOnly: cfg.WorkspaceReadOnly,
		Mounts:            convertWorkflowMounts(cfg.Mounts),
		Ports:             convertWorkflowPorts(cfg.Ports),
		Env:               mergeEnvSlices(envMapToSlice(cfg.Env), params.StepEnv),
		User:              cfg.User,
		RunArgs:           cfg.RunArgs,
		PullPolicy:        cfg.Pull,
		CleanupPolicy:     cfg.Cleanup,
		TTY:               step.Tty,
		Interactive:       step.Interactive,
		Labels: map[string]string{
			container.SandboxLabelType:         container.SandboxTypeWorkflow,
			container.SandboxLabelWorkflow:     params.Workflow,
			container.SandboxLabelWorkflowPath: params.WorkflowPath,
		},
	}
}

// writeEphemeralResult renders captured stdout/stderr from a one-shot step container.
func writeEphemeralResult(params *ContainerStepParams, result *container.EphemeralResult, runErr error) {
	step := params.Step
	if result == nil || step.Tty || step.Interactive {
		return
	}
	writer := stepPkg.NewOutputModeWriter(stepPkg.GetOutputMode(step, params.WorkflowDef), step.Name, stepPkg.GetViewportConfig(step, params.WorkflowDef))
	_, _, _ = writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		if result.Stdout != "" {
			_, _ = stdout.Write([]byte(result.Stdout))
		}
		if result.Stderr != "" {
			_, _ = stderr.Write([]byte(result.Stderr))
		}
		return runErr
	})
}

func mergeWorkflowContainer(base, override *schema.WorkflowContainer) *schema.WorkflowContainer {
	if override == nil {
		return base
	}
	if base == nil {
		return override
	}
	merged := *base
	mergeContainerScalars(&merged, override)
	merged.RuntimeAutoStart = merged.RuntimeAutoStart || override.RuntimeAutoStart
	merged.WorkspaceReadOnly = merged.WorkspaceReadOnly || override.WorkspaceReadOnly
	mergeContainerCollections(&merged, override)
	return &merged
}

// mergeContainerScalars overlays non-empty scalar fields from override onto merged.
func mergeContainerScalars(merged, override *schema.WorkflowContainer) {
	if override.Enabled != nil {
		merged.Enabled = override.Enabled
	}
	if override.Image != "" {
		merged.Image = override.Image
	}
	if override.Shell != "" {
		merged.Shell = override.Shell
	}
	if override.Provider != "" {
		merged.Provider = override.Provider
	}
	if override.Pull != "" {
		merged.Pull = override.Pull
	}
	if override.Workspace != "" {
		merged.Workspace = override.Workspace
	}
	if override.Cleanup != "" {
		merged.Cleanup = override.Cleanup
	}
	if override.User != "" {
		merged.User = override.User
	}
}

// mergeContainerCollections overlays non-empty slice/map fields from override onto merged.
func mergeContainerCollections(merged, override *schema.WorkflowContainer) {
	if len(override.RunArgs) > 0 {
		merged.RunArgs = override.RunArgs
	}
	if len(override.Mounts) > 0 {
		merged.Mounts = override.Mounts
	}
	if len(override.Ports) > 0 {
		merged.Ports = override.Ports
	}
	if len(override.Env) > 0 {
		merged.Env = override.Env
	}
}

func mapHostWorkDirToContainer(hostWorkDir, hostWorkspace, containerWorkspace string) (string, error) {
	if hostWorkDir == "" || hostWorkDir == currentDir {
		return containerWorkspace, nil
	}
	absWorkDir, err := filepath.Abs(hostWorkDir)
	if err != nil {
		return "", err
	}
	absWorkspace, err := filepath.Abs(hostWorkspace)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absWorkspace, absWorkDir)
	if err != nil {
		return "", err
	}
	// filepath.Rel returns OS-native separators; normalize to forward slashes so the
	// workspace-escape guard below is separator-agnostic (it would miss "..\\" on Windows).
	rel = filepath.ToSlash(rel)
	if rel == currentDir {
		return containerWorkspace, nil
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%w: working_directory %q is outside workflow container workspace %q", errUtils.ErrContainerRuntimeOperation, hostWorkDir, hostWorkspace)
	}
	return filepath.ToSlash(filepath.Join(containerWorkspace, rel)), nil
}

func convertWorkflowMounts(mounts []schema.ContainerMount) []container.Mount {
	result := make([]container.Mount, 0, len(mounts))
	for _, mount := range mounts {
		mountType := mount.Type
		if mountType == "" {
			mountType = "bind"
		}
		result = append(result, container.Mount{
			Type:     mountType,
			Source:   expandHome(mount.Source),
			Target:   mount.Target,
			ReadOnly: mount.ReadOnly,
		})
	}
	return result
}

func convertWorkflowPorts(ports []schema.ContainerPort) []container.PortBinding {
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

func envMapToSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(env))
	for _, key := range keys {
		result = append(result, key+"="+env[key])
	}
	return result
}

func mergeEnvSlices(base, overlay []string) []string {
	if len(base) == 0 {
		return append([]string{}, overlay...)
	}
	if len(overlay) == 0 {
		return append([]string{}, base...)
	}
	values := make(map[string]string, len(base)+len(overlay))
	order := make([]string, 0, len(base)+len(overlay))
	set := func(entry string) {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			return
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	for _, entry := range base {
		set(entry)
	}
	for _, entry := range overlay {
		set(entry)
	}
	result := make([]string, 0, len(order))
	for _, key := range order {
		result = append(result, key+"="+values[key])
	}
	return result
}

func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := homedir.Dir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func runtimePreviewName(value string) string {
	return defaultString(value, "docker|podman")
}

func validRuntime(value string) bool {
	return value == "" || value == string(container.TypeDocker) || value == string(container.TypePodman)
}

func validPull(value string) bool {
	return value == "" || value == container.PullMissing || value == container.PullAlways || value == container.PullNever
}

func validCleanup(value string) bool {
	return value == "" || value == container.CleanupAlways || value == container.CleanupOnSuccess || value == container.CleanupNever
}

// Cleanup removes the workflow container according to policy.
func (s *ContainerSession) Cleanup(success bool) error {
	if s == nil || s.backend == nil {
		return nil
	}
	return s.backend.Cleanup(success)
}
