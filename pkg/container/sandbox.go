package container

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	SandboxLabelType         = "com.atmos.type"
	SandboxLabelWorkflow     = "com.atmos.workflow"
	SandboxLabelWorkflowPath = "com.atmos.workflow.path"
	SandboxLabelRunID        = "com.atmos.run.id"
	SandboxLabelWorkspace    = "com.atmos.workspace"
	SandboxTypeWorkflow      = "workflow-sandbox"
)

var sandboxNamePattern = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

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
func StartSandbox(ctx context.Context, config SandboxConfig) (*Sandbox, error) {
	defer perf.Track(nil, "container.StartSandbox")()

	normalizeSandboxConfig(&config)
	if config.Image == "" {
		return nil, fmt.Errorf("%w: workflow container image is required", errUtils.ErrContainerRuntimeOperation)
	}
	if config.DryRun {
		return &Sandbox{config: config}, nil
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

func startSandboxWithRuntime(ctx context.Context, runtime Runtime, config SandboxConfig) (*Sandbox, error) {
	normalizeSandboxConfig(&config)
	cleanupStaleWorkflowSandboxes(ctx, runtime, &config)

	if config.PullPolicy == PullAlways {
		if err := runtime.Pull(ctx, config.Image); err != nil {
			return nil, fmt.Errorf("%w: pull image %q: %w", errUtils.ErrContainerRuntimeOperation, config.Image, err)
		}
	}

	containerID, err := createSandboxContainer(ctx, runtime, &config)
	if err != nil {
		return nil, err
	}
	if err := runtime.Start(ctx, containerID); err != nil {
		_ = runtime.Remove(context.Background(), containerID, true)
		return nil, fmt.Errorf("%w: start workflow sandbox %q: %w", errUtils.ErrContainerRuntimeOperation, containerID, err)
	}

	return &Sandbox{config: config, runtime: runtime, containerID: containerID}, nil
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
	if err == nil || config.PullPolicy != PullMissing {
		if err != nil {
			return "", fmt.Errorf("%w: create workflow sandbox: %w", errUtils.ErrContainerRuntimeOperation, err)
		}
		return containerID, nil
	}
	if pullErr := runtime.Pull(ctx, config.Image); pullErr != nil {
		return "", fmt.Errorf("%w: create workflow sandbox and pull image: %w", errUtils.ErrContainerRuntimeOperation, pullErr)
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
	for _, info := range containers {
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
	status = strings.ToLower(status)
	return strings.Contains(status, "running") || strings.HasPrefix(status, "up ")
}

func sanitizeSandboxName(value string) string {
	value = strings.Trim(sandboxNamePattern.ReplaceAllString(value, "-"), "-.")
	if value == "" {
		return "workflow"
	}
	if len(value) > 48 {
		return value[:48]
	}
	return value
}

// ID returns the runtime container ID. In dry run it returns the generated name.
func (s *Sandbox) ID() string {
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
	cleanupCtx := context.Background()
	_ = s.runtime.Stop(cleanupCtx, s.containerID, 10*time.Second)
	return s.runtime.Remove(cleanupCtx, s.containerID, true)
}
