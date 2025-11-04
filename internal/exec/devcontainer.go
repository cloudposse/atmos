package exec

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	configSeparatorWidth = 90
)

// devcontainerSpinnerModel is a simple spinner model for devcontainer operations.
type devcontainerSpinnerModel struct {
	spinner spinner.Model
	message string
	done    bool
	err     error
}

type devcontainerOpCompleteMsg struct {
	err error
}

//nolint:gocritic // bubbletea models must be passed by value
func (m devcontainerSpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

//nolint:gocritic // bubbletea models must be passed by value
func (m devcontainerSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case devcontainerOpCompleteMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

//nolint:gocritic // bubbletea models must be passed by value
func (m devcontainerSpinnerModel) View() string {
	if m.done {
		if m.err != nil {
			return ""
		}
		return fmt.Sprintf("\r%s %s\n", theme.Styles.Checkmark.String(), m.message)
	}
	return fmt.Sprintf("\r%s %s", m.spinner.View(), m.message)
}

func newDevcontainerSpinner(message string) devcontainerSpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.Styles.Link
	return devcontainerSpinnerModel{
		spinner: s,
		message: message,
	}
}

// ExecuteDevcontainerList lists all available devcontainers.
func ExecuteDevcontainerList(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerList")()

	// Reload config to ensure we have the latest with all fields populated.
	// This is necessary because the config passed via SetAtmosConfig may be incomplete.
	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	configs, err := devcontainer.LoadAllConfigs(&freshConfig)
	if err != nil {
		return err
	}

	if len(configs) == 0 {
		fmt.Println("No devcontainers configured")
		return nil
	}

	// Print header.
	fmt.Printf("%-20s %-40s %-30s\n", "NAME", "IMAGE", "PORTS")
	fmt.Println(strings.Repeat("-", configSeparatorWidth))

	// Print each devcontainer.
	for name, config := range configs {
		image := config.Image
		if image == "" && config.Build != nil {
			image = fmt.Sprintf("(build: %s)", config.Build.Dockerfile)
		}

		ports, _ := devcontainer.ParsePorts(config.ForwardPorts, config.PortsAttributes)
		portsStr := devcontainer.FormatPortBindings(ports)

		fmt.Printf("%-20s %-40s %-30s\n", name, image, portsStr)
	}

	return nil
}

// ExecuteDevcontainerStart starts a devcontainer with optional identity.
func ExecuteDevcontainerStart(atmosConfig *schema.AtmosConfiguration, name, instance, identityName string) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerStart")()

	ctx := context.Background()
	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	config, settings, err := devcontainer.LoadConfig(&freshConfig, name)
	if err != nil {
		return err
	}

	// Inject identity environment variables if identity is specified.
	if identityName != "" {
		if err := injectIdentityEnvironment(ctx, config, identityName); err != nil {
			return err
		}
	}

	runtime, err := devcontainer.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	containerName, err := devcontainer.GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	filters := map[string]string{"name": containerName}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return fmt.Errorf("%w: failed to list containers: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	if len(containers) == 0 {
		params := &containerParams{
			ctx:           ctx,
			runtime:       runtime,
			config:        config,
			containerName: containerName,
			name:          name,
			instance:      instance,
		}
		return createAndStartNewContainer(params)
	}

	return startExistingContainer(ctx, runtime, &containers[0], containerName)
}

// ExecuteDevcontainerStop stops a devcontainer.
func ExecuteDevcontainerStop(atmosConfig *schema.AtmosConfiguration, name, instance string, timeout int) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerStop")()

	ctx := context.Background()

	// Reload config to get fresh devcontainer data.
	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	// Load settings to get runtime.
	_, settings, err := devcontainer.LoadConfig(&freshConfig, name)
	if err != nil {
		return err
	}

	// Detect runtime.
	runtime, err := devcontainer.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	// Generate container name.
	containerName, err := devcontainer.GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	// Check if container exists.
	filters := map[string]string{
		"name": containerName,
	}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return fmt.Errorf("%w: failed to list containers: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	if len(containers) == 0 {
		return fmt.Errorf("%w: container %s not found", errUtils.ErrDevcontainerNotFound, containerName)
	}

	container := containers[0]

	// Check if already stopped.
	if !strings.Contains(strings.ToLower(container.Status), "running") {
		fmt.Fprintf(os.Stderr, "Container %s is already stopped\n", containerName)
		return nil
	}

	// Stop the container with spinner.
	return runWithSpinner(fmt.Sprintf("Stopping container %s", containerName), func() error {
		stopTimeout := time.Duration(timeout) * time.Second
		if err := runtime.Stop(ctx, container.ID, stopTimeout); err != nil {
			return fmt.Errorf("%w: failed to stop container: %w", errUtils.ErrContainerRuntimeOperation, err)
		}
		return nil
	})
}

// ExecuteDevcontainerAttach attaches to a running devcontainer.
// TODO: Add --identity flag support. When implemented, ENV file paths from identity
// must be resolved relative to container paths (e.g., /localhost or bind mount location),
// not host paths, since the container runs in its own filesystem namespace.
func ExecuteDevcontainerAttach(atmosConfig *schema.AtmosConfiguration, name, instance string) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerAttach")()

	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	config, settings, err := devcontainer.LoadConfig(&freshConfig, name)
	if err != nil {
		return err
	}

	runtime, err := devcontainer.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	containerName, err := devcontainer.GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	ctx := context.Background()
	containerInfo, err := findAndStartContainer(ctx, runtime, containerName)
	if err != nil {
		return err
	}

	return attachToContainer(ctx, runtime, containerInfo, config, containerName)
}

func findAndStartContainer(ctx context.Context, runtime container.Runtime, containerName string) (*container.Info, error) {
	filters := map[string]string{"name": containerName}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list containers: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("%w: container %s not found", errUtils.ErrDevcontainerNotFound, containerName)
	}

	containerInfo := &containers[0]

	if !isContainerRunning(containerInfo.Status) {
		if err := startContainerForAttach(ctx, runtime, containerInfo, containerName); err != nil {
			return nil, err
		}
	}

	return containerInfo, nil
}

func startContainerForAttach(ctx context.Context, runtime container.Runtime, containerInfo *container.Info, containerName string) error {
	fmt.Fprintf(os.Stderr, "Starting container %s...\n", containerName)
	if err := runtime.Start(ctx, containerInfo.ID); err != nil {
		return fmt.Errorf("%w: failed to start container: %w", errUtils.ErrContainerRuntimeOperation, err)
	}
	return nil
}

func attachToContainer(ctx context.Context, runtime container.Runtime, containerInfo *container.Info, config *devcontainer.Config, containerName string) error {
	fmt.Fprintf(os.Stderr, "Attaching to container %s...\n", containerName)

	// Warn about masking limitations in interactive TTY sessions.
	maskingEnabled := viper.GetBool("mask")
	if maskingEnabled {
		log.Debug("Interactive TTY session - output masking is not available due to TTY limitations")
	}

	shellArgs := getShellArgs(config.UserEnvProbe)
	attachOpts := &container.AttachOptions{ShellArgs: shellArgs}

	// IO streams are nil in opts, will default to iolib.Data/UI in runtime.
	return runtime.Attach(ctx, containerInfo.ID, attachOpts)
}

func execInContainer(ctx context.Context, runtime container.Runtime, containerID string, interactive bool, command []string) error {
	// Check if masking is enabled and warn about interactive mode limitations.
	maskingEnabled := viper.GetBool("mask")
	if interactive && maskingEnabled {
		log.Debug("Interactive TTY mode enabled - output masking is not available due to TTY limitations")
	}

	execOpts := &container.ExecOptions{
		Tty:          interactive, // TTY mode for interactive sessions.
		AttachStdin:  interactive, // Attach stdin only in interactive mode.
		AttachStdout: true,
		AttachStderr: true,
		// IO streams are nil, will default to iolib.Data/UI in runtime.
	}

	return runtime.Exec(ctx, containerID, command, execOpts)
}

func getShellArgs(userEnvProbe string) []string {
	if userEnvProbe == "loginShell" || userEnvProbe == "loginInteractiveShell" {
		return []string{"-l"}
	}
	return nil
}

// ExecuteDevcontainerExec executes a command in a running devcontainer.
// TODO: Add --identity flag support. When implemented, ENV file paths from identity
// must be resolved relative to container paths (e.g., /localhost or bind mount location),
// not host paths, since the container runs in its own filesystem namespace.
func ExecuteDevcontainerExec(atmosConfig *schema.AtmosConfiguration, name, instance string, interactive bool, command []string) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerExec")()

	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	_, settings, err := devcontainer.LoadConfig(&freshConfig, name)
	if err != nil {
		return err
	}

	runtime, err := devcontainer.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	containerName, err := devcontainer.GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	ctx := context.Background()
	containerInfo, err := findAndStartContainer(ctx, runtime, containerName)
	if err != nil {
		return err
	}

	return execInContainer(ctx, runtime, containerInfo.ID, interactive, command)
}

// ExecuteDevcontainerRemove removes a devcontainer.
func ExecuteDevcontainerRemove(atmosConfig *schema.AtmosConfiguration, name, instance string, force bool) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerRemove")()

	// TODO: Implement devcontainer remove.
	ctx := context.Background()
	_ = ctx
	_ = name
	_ = instance
	_ = force
	return fmt.Errorf("%w: devcontainer remove not yet implemented", errUtils.ErrNotImplemented)
}

// ExecuteDevcontainerConfig shows the configuration for a devcontainer.
func ExecuteDevcontainerConfig(atmosConfig *schema.AtmosConfiguration, name string) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerConfig")()

	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	config, settings, err := devcontainer.LoadConfig(&freshConfig, name)
	if err != nil {
		return err
	}

	printDevcontainerSettings(settings)
	printDevcontainerBasicInfo(config)
	printDevcontainerBuildInfo(config)
	printDevcontainerWorkspaceInfo(config)
	printDevcontainerMounts(config)
	printDevcontainerPorts(config)
	printDevcontainerEnv(config)
	printDevcontainerRunArgs(config)
	printDevcontainerRemoteUser(config)

	return nil
}

func printDevcontainerSettings(settings *devcontainer.Settings) {
	if settings.Runtime != "" {
		fmt.Printf("Runtime: %s\n\n", settings.Runtime)
	}
}

func printDevcontainerBasicInfo(config *devcontainer.Config) {
	fmt.Printf("Name: %s\n", config.Name)
	fmt.Printf("Image: %s\n", config.Image)
}

func printDevcontainerBuildInfo(config *devcontainer.Config) {
	if config.Build == nil {
		return
	}

	fmt.Println("\nBuild:")
	fmt.Printf("  Dockerfile: %s\n", config.Build.Dockerfile)
	fmt.Printf("  Context: %s\n", config.Build.Context)

	if len(config.Build.Args) > 0 {
		fmt.Println("  Args:")
		for k, v := range config.Build.Args {
			fmt.Printf("    %s: %s\n", k, v)
		}
	}
}

func printDevcontainerWorkspaceInfo(config *devcontainer.Config) {
	if config.WorkspaceFolder != "" {
		fmt.Printf("\nWorkspace Folder: %s\n", config.WorkspaceFolder)
	}

	if config.WorkspaceMount != "" {
		fmt.Printf("Workspace Mount: %s\n", config.WorkspaceMount)
	}
}

func printDevcontainerMounts(config *devcontainer.Config) {
	if len(config.Mounts) == 0 {
		return
	}

	fmt.Println("\nMounts:")
	for _, mount := range config.Mounts {
		fmt.Printf("  - %s\n", mount)
	}
}

func printDevcontainerPorts(config *devcontainer.Config) {
	ports, _ := devcontainer.ParsePorts(config.ForwardPorts, config.PortsAttributes)
	if len(ports) == 0 {
		return
	}

	fmt.Println("\nForward Ports:")
	for _, port := range ports {
		if port.HostPort == port.ContainerPort {
			fmt.Printf("  - %d\n", port.ContainerPort)
		} else {
			fmt.Printf("  - %d:%d\n", port.HostPort, port.ContainerPort)
		}
	}
}

func printDevcontainerEnv(config *devcontainer.Config) {
	if len(config.ContainerEnv) == 0 {
		return
	}

	fmt.Println("\nEnvironment Variables:")
	for k, v := range config.ContainerEnv {
		fmt.Printf("  %s: %s\n", k, v)
	}
}

func printDevcontainerRunArgs(config *devcontainer.Config) {
	if len(config.RunArgs) == 0 {
		return
	}

	fmt.Println("\nRun Arguments:")
	for _, arg := range config.RunArgs {
		fmt.Printf("  - %s\n", arg)
	}
}

func printDevcontainerRemoteUser(config *devcontainer.Config) {
	if config.RemoteUser != "" {
		fmt.Printf("\nRemote User: %s\n", config.RemoteUser)
	}
}

// ExecuteDevcontainerLogs shows logs from a devcontainer.
func ExecuteDevcontainerLogs(atmosConfig *schema.AtmosConfiguration, name, instance string, follow bool, tail string) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerLogs")()

	// Reload config to ensure we have the latest with all fields populated.
	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	_, settings, err := devcontainer.LoadConfig(&freshConfig, name)
	if err != nil {
		return err
	}

	// Get container runtime.
	runtime, err := devcontainer.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	// Generate container name.
	containerName, err := devcontainer.GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	// Get container info to verify it exists.
	ctx := context.Background()
	_, err = runtime.Inspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("%w: container %s not found", errUtils.ErrContainerNotFound, containerName)
	}

	// Show logs using default iolib.Data/UI channels.
	return runtime.Logs(ctx, containerName, follow, tail, nil, nil)
}

// ExecuteDevcontainerRebuild rebuilds a devcontainer from scratch.
func ExecuteDevcontainerRebuild(atmosConfig *schema.AtmosConfiguration, name, instance, identityName string, noPull bool) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerRebuild")()

	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	config, settings, err := devcontainer.LoadConfig(&freshConfig, name)
	if err != nil {
		return err
	}

	// Inject identity environment variables if identity is specified.
	if identityName != "" {
		ctx := context.Background()
		if err := injectIdentityEnvironment(ctx, config, identityName); err != nil {
			return err
		}
	}

	runtime, err := devcontainer.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	ctx := context.Background()
	containerName, err := devcontainer.GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	params := &rebuildParams{
		ctx:           ctx,
		runtime:       runtime,
		config:        config,
		containerName: containerName,
		name:          name,
		instance:      instance,
		noPull:        noPull,
	}
	return rebuildContainer(params)
}

type rebuildParams struct {
	ctx           context.Context
	runtime       container.Runtime
	config        *devcontainer.Config
	containerName string
	name          string
	instance      string
	noPull        bool
}

func rebuildContainer(p *rebuildParams) error {
	// Stop and remove existing container if it exists.
	if err := stopAndRemoveContainer(p.ctx, p.runtime, p.containerName); err != nil {
		return err
	}

	// Pull latest image unless --no-pull is set.
	if err := pullImageIfNeeded(p.ctx, p.runtime, p.config.Image, p.noPull); err != nil {
		return err
	}

	// Create and start new container.
	params := &containerParams{
		ctx:           p.ctx,
		runtime:       p.runtime,
		config:        p.config,
		containerName: p.containerName,
		name:          p.name,
		instance:      p.instance,
	}
	containerID, err := createContainer(params)
	if err != nil {
		return err
	}

	if err := startContainer(p.ctx, p.runtime, containerID, p.containerName); err != nil {
		return err
	}

	u.PrintfMessageToTUI("\n%s Container %s rebuilt successfully\n", theme.Styles.Checkmark.String(), p.containerName)
	return nil
}
