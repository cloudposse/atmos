//go:build !linting
// +build !linting

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
)

// Default container settings.
const (
	devContainerDir  = "."
	devContainerFile = "devcontainer.json"
	defaultContainer = "geodesic-dev"
)

// Logger setup.
var logger = log.NewWithOptions(os.Stdout, log.Options{
	Level: log.DebugLevel, // Adjust for more/less verbosity
})

// DevContainerSpec represents the configuration of a dev container.
type DevContainerSpec struct {
	Image             string   `json:"image"`
	ContainerName     string   `json:"containerName,omitempty"`
	WorkspaceFolder   string   `json:"workspaceFolder,omitempty"`
	RunArgs           []string `json:"runArgs,omitempty"`
	PostCreateCommand string   `json:"postCreateCommand,omitempty"`
}

func main() {
	logger.Info("Starting DevContainer manager...")
	defer logger.Info("DevContainer session ended")

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		logger.Fatal("Failed to get current directory", "error", err)
	}

	// Load devcontainer.json
	devContainerPath := filepath.Join(cwd, devContainerDir, devContainerFile)
	logger.Debug("Loading Dev Container configuration", "path", devContainerPath)

	devContainerConfig, err := loadDevContainerConfig(devContainerPath)
	if err != nil {
		logger.Fatal("Error loading devcontainer.json", "error", err)
	}

	// Check if container exists
	if containerExists(devContainerConfig.ContainerName) {
		logger.Info("Existing container found. Starting it...", "container", devContainerConfig.ContainerName)
		startContainer(devContainerConfig.ContainerName)
		runPostCreateCommand(devContainerConfig)
		attachToContainer(devContainerConfig.ContainerName)

		return
	}

	// Pull image if needed
	logger.Info("Pulling container image...", "image", devContainerConfig.Image)
	pullImage(devContainerConfig.Image)

	// Create and start container
	logger.Info("Creating and starting container...", "container", devContainerConfig.ContainerName)
	createContainer(devContainerConfig, cwd)
	startContainer(devContainerConfig.ContainerName)

	// Run post-create command if specified
	runPostCreateCommand(devContainerConfig)

	// Attach to container interactively
	attachToContainer(devContainerConfig.ContainerName)
}

// loadDevContainerConfig loads the devcontainer.json configuration.
func loadDevContainerConfig(path string) (*DevContainerSpec, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config DevContainerSpec

	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		return nil, err
	}

	// Set defaults
	if config.ContainerName == "" {
		config.ContainerName = defaultContainer
	}

	if config.WorkspaceFolder == "" {
		config.WorkspaceFolder = "/workspace"
	}

	logger.Debug("Dev Container configuration loaded", "container", config.ContainerName, "image", config.Image)

	return &config, nil
}

// pullImage pulls the specified container image.
func pullImage(image string) {
	cmd := exec.Command("podman", "pull", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		logger.Fatal("Error pulling image", "image", image, "error", err)
	}
}

// createContainer creates a Podman container.
func createContainer(config *DevContainerSpec, cwd string) {
	if containerExists(config.ContainerName) {
		logger.Warn("Existing container detected. Removing it...", "container", config.ContainerName)
		removeContainer(config.ContainerName)
	}

	args := []string{"run", "-di", "--name", config.ContainerName}
	args = append(args, config.RunArgs...)

	if !containsRunArg(config.RunArgs, "-v") {
		args = append(args, "-v", fmt.Sprintf("%s:%s", cwd, config.WorkspaceFolder))
	}

	if isTerminal() {
		args = append(args, "-t")
	}

	args = append(args, config.Image)

	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		logger.Fatal("Error creating container", "error", err)
	}
}

// containerExists checks if a container exists.
func containerExists(name string) bool {
	cmd := exec.Command("podman", "ps", "-a", "--format", "{{.Names}}")

	output, err := cmd.Output()
	if err != nil {
		logger.Fatal("Error checking containers", "error", err)
	}

	return stringContains(string(output), name)
}

// removeContainer removes an existing container.
func removeContainer(name string) {
	logger.Info("Removing existing container...", "container", name)

	cmd := exec.Command("podman", "rm", "-f", name)
	if err := cmd.Run(); err != nil {
		logger.Warn("Failed to remove container", "container", name, "error", err)
	} else {
		logger.Info("Container removed successfully", "container", name)
	}
}

// startContainer starts a stopped container.
func startContainer(name string) {
	logger.Info("Starting container...", "container", name)

	cmd := exec.Command("podman", "start", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		logger.Fatal("Error starting container", "error", err)
	}
}

// runPostCreateCommand executes the postCreateCommand inside the container.
func runPostCreateCommand(config *DevContainerSpec) {
	if config.PostCreateCommand == "" {
		return
	}

	logger.Info("Running postCreateCommand inside container", "container", config.ContainerName)

	args := []string{"exec", "-i"}
	if isTerminal() {
		args = append(args, "-t")
	}

	args = append(args, config.ContainerName, "sh", "-c", config.PostCreateCommand)

	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		logger.Fatal("Error executing postCreateCommand", "error", err)
	}

	logger.Info("PostCreateCommand executed successfully", "container", config.ContainerName)
}

// attachToContainer attaches to the running container.
func attachToContainer(containerName string) {
	logger.Info("Attaching to container...", "container", containerName)

	args := []string{"exec", "-i"}
	if isTerminal() {
		args = append(args, "-t")
	}

	// Check if TERM is set and pass it to the container
	if term := os.Getenv("TERM"); term != "" {
		log.Debug("Setting ENV", "TERM", term)
		args = append(args, "--env", fmt.Sprintf("TERM=%s", term))
	}

	args = append(args, containerName, "bash", "-l")
	cmd := exec.Command("podman", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fullCommand := fmt.Sprintf("%s %s", cmd.Path, strings.Join(cmd.Args[1:], " "))
	logger.Debug("Executing command", "cmd", fullCommand)

	if err := cmd.Run(); err != nil {
		logger.Fatal("Error attaching to container", "error", err)
	}
}

// isTerminal checks if output is a TTY.
func isTerminal() bool {
	fi, _ := os.Stdout.Stat()
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// containsRunArg checks if runArgs already contain a volume mount.
func containsRunArg(runArgs []string, flag string) bool {
	for _, arg := range runArgs {
		if strings.Contains(arg, flag) {
			return true
		}
	}

	return false
}

// stringContains checks if a substring exists in a string.
func stringContains(s, substr string) bool {
	return len(s) > 0 && substr != "" && strings.Contains(s, substr)
}
