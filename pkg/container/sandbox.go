package container

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	SandboxLabelType         = "tools.atmos.type"
	SandboxLabelWorkflow     = "tools.atmos.workflow"
	SandboxLabelWorkflowPath = "tools.atmos.workflow.path"
	SandboxLabelRunID        = "tools.atmos.run.id"
	SandboxLabelWorkspace    = "tools.atmos.workspace"
	SandboxTypeWorkflow      = "workflow-sandbox"
)

// maxSandboxNameLength bounds the sanitized container name length.
const maxSandboxNameLength = 48

// SandboxConfig describes a long-lived workflow sandbox container.
type SandboxConfig struct {
	Name              string
	Workflow          string
	WorkflowPath      string
	RunID             string
	RuntimeName       string
	RuntimeAutoStart  bool
	Image             string
	WorkspaceHostPath string
	WorkspaceFolder   string
	WorkspaceReadOnly bool
	Mounts            []Mount
	Ports             []PortBinding
	Env               []string
	RuntimeEnv        []string
	User              string
	Labels            map[string]string
	RunArgs           []string
	PullPolicy        string
	CleanupPolicy     string
	DryRun            bool
}

// Sandbox is a started or dry-run workflow sandbox.
type Sandbox struct {
	config      SandboxConfig
	runtime     Runtime
	containerID string
}

// NewWorkflowSandboxConfig builds a sandbox config with stable labels and a
// unique container name for a workflow execution.
func NewWorkflowSandboxConfig(workflow, workflowPath, workspaceHostPath string) SandboxConfig {
	defer perf.Track(nil, "container.NewWorkflowSandboxConfig")()

	runID := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	name := fmt.Sprintf("atmos-workflow-%s-%s", sanitizeSandboxName(workflow), sanitizeSandboxName(runID))
	labels := map[string]string{
		SandboxLabelType:         SandboxTypeWorkflow,
		SandboxLabelWorkflow:     workflow,
		SandboxLabelWorkflowPath: workflowPath,
		SandboxLabelRunID:        runID,
		SandboxLabelWorkspace:    workspaceHostPath,
	}

	return SandboxConfig{
		Name:              name,
		Workflow:          workflow,
		WorkflowPath:      workflowPath,
		RunID:             runID,
		WorkspaceHostPath: workspaceHostPath,
		WorkspaceFolder:   "/workspace",
		PullPolicy:        PullMissing,
		CleanupPolicy:     CleanupAlways,
		Labels:            labels,
	}
}

// StartSandbox detects the runtime, cleans up stopped stale sandboxes, creates,
// and starts the workflow sandbox.
func StartSandbox(ctx context.Context, config *SandboxConfig) (*Sandbox, error) {
	defer perf.Track(nil, "container.StartSandbox")()

	if config == nil {
		return nil, errUtils.ErrNilParam
	}
	normalizeSandboxConfig(config)
	if config.Image == "" {
		return nil, fmt.Errorf("%w: workflow container image is required", errUtils.ErrContainerRuntimeOperation)
	}
	if config.DryRun {
		return &Sandbox{config: *config}, nil
	}

	runtime, err := DetectRuntimeWithPreferenceAndRecovery(ctx, config.RuntimeName, config.RuntimeAutoStart)
	if err != nil {
		return nil, err
	}
	if setter, ok := runtime.(EnvSetter); ok {
		setter.SetEnv(config.RuntimeEnv)
	}

	return startSandboxWithRuntime(ctx, runtime, config)
}

func startSandboxWithRuntime(ctx context.Context, runtime Runtime, config *SandboxConfig) (*Sandbox, error) {
	normalizeSandboxConfig(config)
	cleanupStaleWorkflowSandboxes(ctx, runtime, config)

	if config.PullPolicy == PullAlways {
		if err := runtime.Pull(ctx, config.Image); err != nil {
			return nil, fmt.Errorf("%w: pull image %q: %w", errUtils.ErrContainerRuntimeOperation, config.Image, err)
		}
	}

	containerID, err := createSandboxContainer(ctx, runtime, config)
	if err != nil {
		return nil, err
	}
	if err := runtime.Start(ctx, containerID); err != nil {
		_ = runtime.Remove(context.Background(), containerID, true)
		return nil, fmt.Errorf("%w: start workflow sandbox %q: %w", errUtils.ErrContainerRuntimeOperation, containerID, err)
	}

	return &Sandbox{config: *config, runtime: runtime, containerID: containerID}, nil
}

func normalizeSandboxConfig(config *SandboxConfig) {
	if config.Name == "" {
		config.Name = fmt.Sprintf("atmos-workflow-%s", sanitizeSandboxName(config.RunID))
	}
	if config.WorkspaceFolder == "" {
		config.WorkspaceFolder = "/workspace"
	}
	if config.PullPolicy == "" {
		config.PullPolicy = PullMissing
	}
	if config.CleanupPolicy == "" {
		config.CleanupPolicy = CleanupAlways
	}
	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	config.Labels[SandboxLabelType] = SandboxTypeWorkflow
	if config.Workflow != "" {
		config.Labels[SandboxLabelWorkflow] = config.Workflow
	}
	if config.WorkflowPath != "" {
		config.Labels[SandboxLabelWorkflowPath] = config.WorkflowPath
	}
	if config.RunID != "" {
		config.Labels[SandboxLabelRunID] = config.RunID
	}
	if config.WorkspaceHostPath != "" {
		config.Labels[SandboxLabelWorkspace] = config.WorkspaceHostPath
	}
}

func createSandboxContainer(ctx context.Context, runtime Runtime, config *SandboxConfig) (string, error) {
	createConfig := buildSandboxCreateConfig(config)
	containerID, err := runtime.Create(ctx, createConfig)
	if err == nil {
		return containerID, nil
	}
	// Only the missing-image case is recoverable by pulling. Any other create
	// failure (bad mount, invalid arg, daemon error) must surface as-is — pulling
	// then would mask the real cause behind a misleading registry error.
	if config.PullPolicy != PullMissing || !IsImageMissingError(err) {
		return "", fmt.Errorf("%w: create workflow sandbox: %w", errUtils.ErrContainerRuntimeOperation, err)
	}
	createErr := err
	if pullErr := runtime.Pull(ctx, config.Image); pullErr != nil {
		return "", fmt.Errorf(
			"%w: create workflow sandbox and pull image: %w",
			errUtils.ErrContainerRuntimeOperation,
			errors.Join(createErr, pullErr),
		)
	}
	containerID, err = runtime.Create(ctx, createConfig)
	if err != nil {
		return "", fmt.Errorf("%w: create workflow sandbox after pull: %w", errUtils.ErrContainerRuntimeOperation, err)
	}
	return containerID, nil
}

func buildSandboxCreateConfig(config *SandboxConfig) *CreateConfig {
	mounts := append([]Mount{}, config.Mounts...)
	if config.WorkspaceHostPath != "" {
		mounts = append(mounts, Mount{
			Type:     "bind",
			Source:   config.WorkspaceHostPath,
			Target:   config.WorkspaceFolder,
			ReadOnly: config.WorkspaceReadOnly,
		})
	}

	return &CreateConfig{
		Name:            config.Name,
		Image:           config.Image,
		WorkspaceFolder: config.WorkspaceFolder,
		Mounts:          mounts,
		Ports:           config.Ports,
		User:            config.User,
		Labels:          config.Labels,
		RunArgs:         config.RunArgs,
		OverrideCommand: true,
	}
}

func cleanupStaleWorkflowSandboxes(ctx context.Context, runtime Runtime, config *SandboxConfig) {
	containers, err := runtime.List(ctx, map[string]string{"label": fmt.Sprintf("%s=%s", SandboxLabelType, SandboxTypeWorkflow)})
	if err != nil {
		return
	}
	for i := range containers {
		info := &containers[i]
		if !matchesSandboxLabels(info.Labels, config) || isContainerRunning(info.Status) {
			continue
		}
		id := info.ID
		if id == "" {
			id = info.Name
		}
		if id != "" {
			_ = runtime.Remove(ctx, id, true)
		}
	}
}

func matchesSandboxLabels(labels map[string]string, config *SandboxConfig) bool {
	if labels == nil || labels[SandboxLabelType] != SandboxTypeWorkflow {
		return false
	}
	if config.Workflow != "" && labels[SandboxLabelWorkflow] != config.Workflow {
		return false
	}
	if config.WorkflowPath != "" && labels[SandboxLabelWorkflowPath] != config.WorkflowPath {
		return false
	}
	if config.WorkspaceHostPath != "" && labels[SandboxLabelWorkspace] != config.WorkspaceHostPath {
		return false
	}
	return true
}

func isContainerRunning(status string) bool {
	return IsContainerRunning(status)
}

func sanitizeSandboxName(value string) string {
	return sanitizeName(value, "workflow", maxSandboxNameLength)
}

// ID returns the runtime container ID. In dry run it returns the generated name.
func (s *Sandbox) ID() string {
	defer perf.Track(nil, "container.Sandbox.ID")()

	if s == nil {
		return ""
	}
	if s.containerID != "" {
		return s.containerID
	}
	return s.config.Name
}

// Name returns the generated sandbox container name.
func (s *Sandbox) Name() string {
	defer perf.Track(nil, "container.Sandbox.Name")()

	if s == nil {
		return ""
	}
	return s.config.Name
}

// Exec runs a command inside the sandbox.
func (s *Sandbox) Exec(ctx context.Context, command []string, opts *ExecOptions) error {
	defer perf.Track(nil, "container.Sandbox.Exec")()

	if s == nil {
		return errUtils.ErrNilParam
	}
	if s.config.DryRun {
		return nil
	}
	return s.runtime.Exec(ctx, s.containerID, command, opts)
}

// Cleanup removes the sandbox according to the configured cleanup policy.
func (s *Sandbox) Cleanup(success bool) error {
	defer perf.Track(nil, "container.Sandbox.Cleanup")()

	if s == nil || s.config.DryRun {
		return nil
	}
	switch s.config.CleanupPolicy {
	case CleanupNever:
		return nil
	case CleanupOnSuccess:
		if !success {
			return nil
		}
	}
	// Force-remove directly (docker/podman rm -f) instead of a graceful Stop first.
	// The sandbox's PID 1 is `/bin/sh -c "sleep infinity"`, and the kernel does not
	// apply default signal dispositions to PID 1, so a graceful Stop's SIGTERM is
	// ignored and `docker stop` blocks for the full grace period before SIGKILL.
	// A throwaway sandbox has nothing to shut down gracefully, so skip Stop and let
	// force-remove SIGKILL immediately — turning a ~10s teardown into a near-instant one.
	cleanupCtx := context.Background()
	return s.runtime.Remove(cleanupCtx, s.containerID, true)
}
