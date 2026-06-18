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
	"github.com/cloudposse/atmos/pkg/container"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const workflowContainerDefaultShell = "/bin/sh"

// WorkflowSandbox owns the long-lived container-backed sandbox for a workflow run.
type WorkflowSandbox struct {
	sandbox       sandboxContainer
	config        *schema.WorkflowContainer
	hostWorkspace string
}

type sandboxContainer interface {
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
			resolvedBasePath = "."
		}
		workDir = filepath.Join(resolvedBasePath, workDir)
	}
	return workDir
}

// StepContainerDisabled reports whether the step explicitly opts out of the workflow sandbox.
func StepContainerDisabled(step *schema.WorkflowStep) bool {
	return step.Container != nil && !step.Container.IsEnabled()
}

// StepContainerOverride reports whether the step has an enabled container override.
func StepContainerOverride(step *schema.WorkflowStep) bool {
	return step.Container != nil && step.Container.IsEnabled()
}

// StartWorkflowSandbox starts the workflow-level sandbox if configured.
func StartWorkflowSandbox(ctx context.Context, workflow, workflowPath, basePath string, workflowDef *schema.WorkflowDefinition, runtimeEnv []string, dryRun bool) (*WorkflowSandbox, error) {
	if workflowDef == nil || workflowDef.Container == nil || !workflowDef.Container.IsEnabled() {
		return nil, nil
	}

	hostWorkspace, err := workflowSandboxHostWorkspace(workflowDef, basePath)
	if err != nil {
		return nil, err
	}
	cfg, err := buildSandboxConfig(workflowDef.Container, workflow, workflowPath, hostWorkspace, runtimeEnv, dryRun)
	if err != nil {
		return nil, err
	}
	sandbox, err := container.StartSandbox(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if dryRun {
		ui.Writef("%s create %s %s\n", runtimePreviewName(cfg.RuntimeName), cfg.Name, cfg.Image)
	}
	return &WorkflowSandbox{sandbox: sandbox, config: workflowDef.Container, hostWorkspace: hostWorkspace}, nil
}

func workflowSandboxHostWorkspace(workflowDef *schema.WorkflowDefinition, basePath string) (string, error) {
	workDir := strings.TrimSpace(workflowDef.WorkingDirectory)
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	} else if !filepath.IsAbs(workDir) {
		if strings.TrimSpace(basePath) == "" {
			basePath = "."
		}
		workDir = filepath.Join(basePath, workDir)
	}
	return filepath.Abs(workDir)
}

func buildSandboxConfig(cfg *schema.WorkflowContainer, workflow, workflowPath, hostWorkspace string, runtimeEnv []string, dryRun bool) (container.SandboxConfig, error) {
	if strings.TrimSpace(cfg.Image) == "" {
		return container.SandboxConfig{}, fmt.Errorf("%w: workflow container image is required", errUtils.ErrContainerRuntimeOperation)
	}
	if !validRuntime(cfg.Runtime) {
		return container.SandboxConfig{}, fmt.Errorf("%w: workflow container runtime must be docker, podman, or empty", errUtils.ErrContainerRuntimeOperation)
	}
	if !validPull(cfg.Pull) {
		return container.SandboxConfig{}, fmt.Errorf("%w: workflow container pull must be missing, always, never, or empty", errUtils.ErrContainerRuntimeOperation)
	}
	if !validCleanup(cfg.Cleanup) {
		return container.SandboxConfig{}, fmt.Errorf("%w: workflow container cleanup must be always, on_success, never, or empty", errUtils.ErrContainerRuntimeOperation)
	}

	sandboxCfg := container.NewWorkflowSandboxConfig(workflow, workflowPath, hostWorkspace)
	sandboxCfg.RuntimeName = cfg.Runtime
	sandboxCfg.RuntimeAutoStart = cfg.RuntimeAutoStart
	sandboxCfg.Image = cfg.Image
	sandboxCfg.WorkspaceFolder = defaultString(cfg.Workspace, "/workspace")
	sandboxCfg.WorkspaceReadOnly = cfg.WorkspaceReadOnly
	sandboxCfg.Mounts = convertWorkflowMounts(cfg.Mounts)
	sandboxCfg.Ports = convertWorkflowPorts(cfg.Ports)
	sandboxCfg.Env = envMapToSlice(cfg.Env)
	sandboxCfg.RuntimeEnv = runtimeEnv
	sandboxCfg.User = cfg.User
	sandboxCfg.RunArgs = cfg.RunArgs
	sandboxCfg.PullPolicy = cfg.Pull
	sandboxCfg.CleanupPolicy = cfg.Cleanup
	sandboxCfg.DryRun = dryRun
	return sandboxCfg, nil
}

// ExecShell runs a shell command inside the workflow sandbox.
func (s *WorkflowSandbox) ExecShell(ctx context.Context, step *schema.WorkflowStep, workflowDef *schema.WorkflowDefinition, hostWorkDir, command string, stepEnv []string) error {
	if s == nil || s.sandbox == nil {
		return errUtils.ErrNilParam
	}
	containerWorkDir, err := mapHostWorkDirToContainer(hostWorkDir, s.hostWorkspace, defaultString(s.config.Workspace, "/workspace"))
	if err != nil {
		return err
	}
	shell := defaultString(s.config.Shell, workflowContainerDefaultShell)
	env := mergeEnvSlices(envMapToSlice(s.config.Env), stepEnv)
	cmd := []string{shell, "-lc", command}
	if s.sandbox.ID() == s.sandbox.Name() {
		ui.Writef("%s exec %s %s\n", runtimePreviewName(s.config.Runtime), s.sandbox.Name(), strings.Join(cmd, " "))
		return nil
	}

	if step.Tty || step.Interactive {
		return s.sandbox.Exec(ctx, cmd, &container.ExecOptions{
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

	writer := stepPkg.NewOutputModeWriter(stepPkg.GetOutputMode(step, workflowDef), step.Name, stepPkg.GetViewportConfig(step, workflowDef))
	_, _, err = writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		return s.sandbox.Exec(ctx, cmd, &container.ExecOptions{
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

// RunStepContainerOverride runs a shell step in a one-shot step-level sandbox.
func RunStepContainerOverride(ctx context.Context, workflow, workflowPath, basePath string, workflowDef *schema.WorkflowDefinition, step *schema.WorkflowStep, hostWorkDir, command string, stepEnv []string, runtimeEnv []string, dryRun bool) error {
	cfg := mergeWorkflowContainer(workflowDef.Container, step.Container)
	if !cfg.IsEnabled() {
		return nil
	}
	if strings.TrimSpace(cfg.Image) == "" {
		return fmt.Errorf("%w: step container image is required", errUtils.ErrContainerRuntimeOperation)
	}
	containerWorkDir := defaultString(cfg.Workspace, "/workspace")
	hostWorkspace := hostWorkDir
	if hostWorkspace == "" {
		var err error
		hostWorkspace, err = workflowSandboxHostWorkspace(workflowDef, basePath)
		if err != nil {
			return err
		}
	}
	absHostWorkspace, err := filepath.Abs(hostWorkspace)
	if err != nil {
		return err
	}
	env := mergeEnvSlices(envMapToSlice(cfg.Env), stepEnv)
	shell := defaultString(cfg.Shell, workflowContainerDefaultShell)
	ephemeral := &container.EphemeralConfig{
		Name:              fmt.Sprintf("atmos-step-%s", step.Name),
		Image:             cfg.Image,
		Command:           []string{shell, "-lc", command},
		WorkspaceHostPath: absHostWorkspace,
		WorkspaceFolder:   containerWorkDir,
		WorkspaceReadOnly: cfg.WorkspaceReadOnly,
		Mounts:            convertWorkflowMounts(cfg.Mounts),
		Ports:             convertWorkflowPorts(cfg.Ports),
		Env:               env,
		User:              cfg.User,
		RunArgs:           cfg.RunArgs,
		PullPolicy:        cfg.Pull,
		CleanupPolicy:     cfg.Cleanup,
		TTY:               step.Tty,
		Interactive:       step.Interactive,
		Labels: map[string]string{
			container.SandboxLabelType:         container.SandboxTypeWorkflow,
			container.SandboxLabelWorkflow:     workflow,
			container.SandboxLabelWorkflowPath: workflowPath,
		},
	}
	if dryRun {
		ui.Writeln(container.BuildEphemeralPreview(runtimePreviewName(cfg.Runtime), ephemeral))
		return nil
	}
	runtime, err := container.DetectRuntimeWithPreferenceAndRecovery(ctx, cfg.Runtime, cfg.RuntimeAutoStart)
	if err != nil {
		return err
	}
	if setter, ok := runtime.(container.EnvSetter); ok {
		setter.SetEnv(runtimeEnv)
	}
	result, err := container.RunEphemeralContainer(ctx, runtime, ephemeral)
	if result != nil && !step.Tty && !step.Interactive {
		writer := stepPkg.NewOutputModeWriter(stepPkg.GetOutputMode(step, workflowDef), step.Name, stepPkg.GetViewportConfig(step, workflowDef))
		_, _, _ = writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
			if result.Stdout != "" {
				_, _ = stdout.Write([]byte(result.Stdout))
			}
			if result.Stderr != "" {
				_, _ = stderr.Write([]byte(result.Stderr))
			}
			return err
		})
	}
	return err
}

func mergeWorkflowContainer(base, override *schema.WorkflowContainer) *schema.WorkflowContainer {
	if override == nil {
		return base
	}
	if base == nil {
		return override
	}
	merged := *base
	if override.Enabled != nil {
		merged.Enabled = override.Enabled
	}
	if override.Image != "" {
		merged.Image = override.Image
	}
	if override.Shell != "" {
		merged.Shell = override.Shell
	}
	if override.Runtime != "" {
		merged.Runtime = override.Runtime
	}
	merged.RuntimeAutoStart = merged.RuntimeAutoStart || override.RuntimeAutoStart
	if override.Pull != "" {
		merged.Pull = override.Pull
	}
	if override.Workspace != "" {
		merged.Workspace = override.Workspace
	}
	merged.WorkspaceReadOnly = merged.WorkspaceReadOnly || override.WorkspaceReadOnly
	if override.Cleanup != "" {
		merged.Cleanup = override.Cleanup
	}
	if override.User != "" {
		merged.User = override.User
	}
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
	return &merged
}

func mapHostWorkDirToContainer(hostWorkDir, hostWorkspace, containerWorkspace string) (string, error) {
	if hostWorkDir == "" || hostWorkDir == "." {
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
	if rel == "." {
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
	home, err := os.UserHomeDir()
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

// Cleanup removes the workflow sandbox according to policy.
func (s *WorkflowSandbox) Cleanup(success bool) error {
	if s == nil || s.sandbox == nil {
		return nil
	}
	return s.sandbox.Cleanup(success)
}
