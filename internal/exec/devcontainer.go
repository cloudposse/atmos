//nolint:revive // file-length-limit: Reduced from 907 to 689 lines (24% reduction), contains core devcontainer lifecycle management
package exec

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/devcontainer"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pty"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	tableWidth = 92 // Width for devcontainer list table including indicator column.

	errListContainers = "%w: failed to list containers: %w"
)

// DevcontainerExecParams encapsulates parameters for ExecuteDevcontainerExec.
type DevcontainerExecParams struct {
	Name        string
	Instance    string
	Interactive bool
	UsePTY      bool
	Command     []string
}

// ExecuteDevcontainerList lists all available devcontainers with running status.
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
		_ = ui.Infof("No devcontainers configured")
		return nil
	}

	// Get runtime and list running containers.
	runtime, err := devcontainer.DetectRuntime("")
	if err != nil {
		return fmt.Errorf("%w: failed to initialize container runtime: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	ctx := context.Background()
	runningContainers, err := runtime.List(ctx, nil)
	if err != nil {
		return fmt.Errorf(errListContainers, errUtils.ErrContainerRuntimeOperation, err)
	}

	// Build set of running devcontainer names.
	runningNames := make(map[string]bool)
	for _, c := range runningContainers {
		if devcontainer.IsAtmosDevcontainer(c.Name) {
			if name, _ := devcontainer.ParseContainerName(c.Name); name != "" {
				if c.Status == "running" {
					runningNames[name] = true
				}
			}
		}
	}

	// Render the table using lipgloss.
	renderDevcontainerListTable(configs, runningNames)
	return nil
}

// renderDevcontainerListTable renders devcontainer list as a formatted table.
func renderDevcontainerListTable(configs map[string]*devcontainer.Config, runningNames map[string]bool) {
	// Sort names for consistent output.
	var names []string
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)

	// Build table rows.
	var rows []string
	for _, name := range names {
		config := configs[name]

		// Determine status indicator.
		indicator := " "
		if runningNames[name] {
			indicator = theme.Styles.NewVersion.Render("●") // Green dot for running.
		}

		// Get image name.
		image := config.Image
		if image == "" && config.Build != nil {
			image = fmt.Sprintf("(build: %s)", config.Build.Dockerfile)
		}

		// Get ports.
		ports, _ := devcontainer.ParsePorts(config.ForwardPorts, config.PortsAttributes)
		portsStr := devcontainer.FormatPortBindings(ports)

		// Format row.
		row := fmt.Sprintf("%s %-20s %-40s %-30s", indicator, name, image, portsStr)
		rows = append(rows, row)
	}

	// Print header with bold styling.
	headerStyle := lipgloss.NewStyle().Bold(true)
	fmt.Printf("%s %-20s %-40s %-30s\n", " ",
		headerStyle.Render("NAME"),
		headerStyle.Render("IMAGE"),
		headerStyle.Render("PORTS"))

	// Print separator.
	fmt.Println(strings.Repeat("─", tableWidth))

	// Print rows.
	for _, row := range rows {
		fmt.Println(row)
	}
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
		return fmt.Errorf(errListContainers, errUtils.ErrContainerRuntimeOperation, err)
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
		return fmt.Errorf(errListContainers, errUtils.ErrContainerRuntimeOperation, err)
	}

	if len(containers) == 0 {
		return fmt.Errorf("%w: container %s not found", errUtils.ErrDevcontainerNotFound, containerName)
	}

	container := containers[0]

	// Check if already stopped.
	if !strings.Contains(strings.ToLower(container.Status), "running") {
		_ = ui.Infof("Container %s is already stopped", containerName)
		return nil
	}

	// Stop the container with spinner.
	return runWithSpinner(
		fmt.Sprintf("Stopping container %s", containerName),
		fmt.Sprintf("Stopped container %s", containerName),
		func() error {
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
func ExecuteDevcontainerAttach(atmosConfig *schema.AtmosConfiguration, name, instance string, usePTY bool) error {
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

	return attachToContainer(&attachParams{
		ctx:           ctx,
		runtime:       runtime,
		containerInfo: containerInfo,
		config:        config,
		containerName: containerName,
		usePTY:        usePTY,
	})
}

func findAndStartContainer(ctx context.Context, runtime container.Runtime, containerName string) (*container.Info, error) {
	filters := map[string]string{"name": containerName}
	containers, err := runtime.List(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf(errListContainers, errUtils.ErrContainerRuntimeOperation, err)
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
	return runWithSpinner(
		fmt.Sprintf("Starting container %s", containerName),
		fmt.Sprintf("Started container %s", containerName),
		func() error {
			if err := runtime.Start(ctx, containerInfo.ID); err != nil {
				return fmt.Errorf("%w: failed to start container: %w", errUtils.ErrContainerRuntimeOperation, err)
			}
			return nil
		})
}

// attachParams holds parameters for attaching to a container.
type attachParams struct {
	ctx           context.Context
	runtime       container.Runtime
	containerInfo *container.Info
	config        *devcontainer.Config
	containerName string
	usePTY        bool
}

func attachToContainer(params *attachParams) error {
	log.Debug("Attaching to container", "container", params.containerName)

	maskingEnabled := viper.GetBool("mask")

	// PTY mode: Use experimental PTY with masking.
	if params.usePTY {
		if !pty.IsSupported() {
			return fmt.Errorf("%w: only macOS and Linux are supported", errUtils.ErrPTYNotSupported)
		}

		log.Debug("Using experimental PTY mode with masking support")
		shellArgs := getShellArgs(params.config.UserEnvProbe)
		return attachToContainerWithPTY(params.ctx, params.runtime, params.containerInfo.ID, shellArgs, maskingEnabled)
	}

	// Regular mode: Warn about masking limitations in interactive TTY sessions.
	if maskingEnabled {
		log.Debug("Interactive TTY session - output masking is not available due to TTY limitations")
	}

	shellArgs := getShellArgs(params.config.UserEnvProbe)
	attachOpts := &container.AttachOptions{ShellArgs: shellArgs}

	// IO streams are nil in opts, will default to iolib.Data/UI in runtime.
	return params.runtime.Attach(params.ctx, params.containerInfo.ID, attachOpts)
}

// execParams holds parameters for executing commands in a container.
type execParams struct {
	ctx         context.Context
	runtime     container.Runtime
	containerID string
	interactive bool
	usePTY      bool
	command     []string
}

func execInContainer(params *execParams) error {
	maskingEnabled := viper.GetBool("mask")

	// PTY mode: Use experimental PTY with masking.
	if params.usePTY {
		if !pty.IsSupported() {
			return fmt.Errorf("%w: only macOS and Linux are supported", errUtils.ErrPTYNotSupported)
		}

		log.Debug("Using experimental PTY mode with masking support")
		return execInContainerWithPTY(params.ctx, params.runtime, params.containerID, params.command, maskingEnabled)
	}

	// Regular mode (existing behavior).
	if params.interactive && maskingEnabled {
		log.Debug("Interactive TTY mode enabled - output masking is not available due to TTY limitations")
	}

	execOpts := &container.ExecOptions{
		Tty:          params.interactive, // TTY mode for interactive sessions.
		AttachStdin:  params.interactive, // Attach stdin only in interactive mode.
		AttachStdout: true,
		AttachStderr: true,
		// IO streams are nil, will default to iolib.Data/UI in runtime.
	}

	return params.runtime.Exec(params.ctx, params.containerID, params.command, execOpts)
}

// attachToContainerWithPTY attaches to a container using PTY mode with masking support.
// This is an experimental feature that provides TTY functionality while preserving
// output masking capabilities.
func attachToContainerWithPTY(ctx context.Context, runtime container.Runtime, containerID string, shellArgs []string, maskingEnabled bool) error {
	// Get the IO context for masking.
	ioCtx := iolib.GetContext()

	// Determine the runtime binary (docker or podman).
	runtimeInfo, err := runtime.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get runtime info: %w", err)
	}

	runtimeBinary := runtimeInfo.Type

	// Build the runtime attach command with shell.
	// Example: docker exec -it <containerID> /bin/bash -l
	args := []string{"exec", "-it", containerID, "/bin/bash"}
	args = append(args, shellArgs...)

	cmd := exec.Command(runtimeBinary, args...)

	// Configure PTY options with masking.
	ptyOpts := &pty.Options{
		Masker:        ioCtx.Masker(),
		EnableMasking: maskingEnabled,
	}

	// Execute with PTY.
	return pty.ExecWithPTY(ctx, cmd, ptyOpts)
}

// execInContainerWithPTY executes a command using PTY mode with masking support.
// This is an experimental feature that provides TTY functionality while preserving
// output masking capabilities.
func execInContainerWithPTY(ctx context.Context, runtime container.Runtime, containerID string, command []string, maskingEnabled bool) error {
	// Get the IO context for masking.
	ioCtx := iolib.GetContext()

	// Determine the runtime binary (docker or podman).
	runtimeInfo, err := runtime.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get runtime info: %w", err)
	}

	runtimeBinary := runtimeInfo.Type

	// Build the runtime exec command.
	// Example: docker exec -it <containerID> <command...>
	args := []string{"exec", "-it", containerID}
	args = append(args, command...)

	cmd := exec.Command(runtimeBinary, args...)

	// Configure PTY options with masking.
	ptyOpts := &pty.Options{
		Masker:        ioCtx.Masker(),
		EnableMasking: maskingEnabled,
	}

	// Execute with PTY.
	return pty.ExecWithPTY(ctx, cmd, ptyOpts)
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
func ExecuteDevcontainerExec(atmosConfig *schema.AtmosConfiguration, params DevcontainerExecParams) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerExec")()

	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	_, settings, err := devcontainer.LoadConfig(&freshConfig, params.Name)
	if err != nil {
		return err
	}

	runtime, err := devcontainer.DetectRuntime(settings.Runtime)
	if err != nil {
		return err
	}

	containerName, err := devcontainer.GenerateContainerName(params.Name, params.Instance)
	if err != nil {
		return err
	}

	ctx := context.Background()
	containerInfo, err := findAndStartContainer(ctx, runtime, containerName)
	if err != nil {
		return err
	}

	return execInContainer(&execParams{
		ctx:         ctx,
		runtime:     runtime,
		containerID: containerInfo.ID,
		interactive: params.Interactive,
		usePTY:      params.UsePTY,
		command:     params.Command,
	})
}

// ExecuteDevcontainerRemove removes a devcontainer by name and instance.
// The operation is idempotent - returns nil if the container does not exist.
//
// Reloads configuration, detects the container runtime, and generates the container name.
// Fails if the container is running unless force is true. When force is true, stops a
// running container before removal. Returns relevant errors for runtime or config failures.
//
// Parameters:
//   - atmosConfig: Atmos configuration for performance tracking
//   - name: Devcontainer name from configuration
//   - instance: Instance identifier (e.g., "default", "prod")
//   - force: If true, stops running containers before removal; if false, fails on running containers
func ExecuteDevcontainerRemove(atmosConfig *schema.AtmosConfiguration, name, instance string, force bool) error {
	defer perf.Track(atmosConfig, "exec.ExecuteDevcontainerRemove")()

	// Reload config to ensure we have the latest with all fields populated.
	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	_, settings, err := devcontainer.LoadConfig(&freshConfig, name)
	if err != nil {
		return err
	}

	// Initialize container runtime.
	runtime, err := devcontainer.DetectRuntime(settings.Runtime)
	if err != nil {
		return fmt.Errorf("%w: failed to initialize container runtime: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	ctx := context.Background()

	// Generate container name.
	containerName, err := devcontainer.GenerateContainerName(name, instance)
	if err != nil {
		return err
	}

	// Check if container exists.
	containerInfo, err := runtime.Inspect(ctx, containerName)
	if err != nil {
		// Container doesn't exist - nothing to remove, consider this success.
		return nil
	}

	// Stop container if running and force=false.
	if isContainerRunning(containerInfo.Status) && !force {
		return fmt.Errorf("%w: %s, use --force to remove", errUtils.ErrContainerRunning, containerName)
	}

	// Stop if running.
	if isContainerRunning(containerInfo.Status) {
		if err := stopContainerIfRunning(ctx, runtime, containerInfo); err != nil {
			return err
		}
	}

	// Remove the container.
	return removeContainer(ctx, runtime, containerInfo, containerName)
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

// GenerateNewDevcontainerInstance generates a new unique instance name by finding
// existing containers for the given devcontainer name and incrementing the highest number.
// Pattern: {baseInstance}-1, {baseInstance}-2, etc.
// Returns the new instance name (e.g., "default-1", "default-2").
func GenerateNewDevcontainerInstance(atmosConfig *schema.AtmosConfiguration, name, baseInstance string) (string, error) {
	defer perf.Track(atmosConfig, "exec.GenerateNewDevcontainerInstance")()

	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return "", err
	}

	_, settings, err := devcontainer.LoadConfig(&freshConfig, name)
	if err != nil {
		return "", err
	}

	runtime, err := devcontainer.DetectRuntime(settings.Runtime)
	if err != nil {
		return "", fmt.Errorf("%w: failed to initialize container runtime: %w", errUtils.ErrContainerRuntimeOperation, err)
	}

	ctx := context.Background()
	if baseInstance == "" {
		baseInstance = devcontainer.DefaultInstance
	}

	containers, err := runtime.List(ctx, nil)
	if err != nil {
		return "", fmt.Errorf(errListContainers, errUtils.ErrContainerRuntimeOperation, err)
	}

	maxNumber := findMaxInstanceNumber(containers, name, baseInstance)
	return fmt.Sprintf("%s-%d", baseInstance, maxNumber+1), nil
}

// findMaxInstanceNumber finds the highest instance number for a given devcontainer name and base instance.
func findMaxInstanceNumber(containers []container.Info, name, baseInstance string) int {
	maxNumber := 0
	basePattern := fmt.Sprintf("%s-", baseInstance)

	for _, c := range containers {
		parsedName, parsedInstance := devcontainer.ParseContainerName(c.Name)
		if parsedName != name {
			continue
		}

		if strings.HasPrefix(parsedInstance, basePattern) {
			numberStr := strings.TrimPrefix(parsedInstance, basePattern)
			var number int
			if _, err := fmt.Sscanf(numberStr, "%d", &number); err == nil {
				if number > maxNumber {
					maxNumber = number
				}
			}
		}
	}

	return maxNumber
}
