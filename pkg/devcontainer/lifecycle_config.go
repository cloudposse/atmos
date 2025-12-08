package devcontainer

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ShowConfig shows the configuration for a devcontainer.
func (m *Manager) ShowConfig(atmosConfig *schema.AtmosConfiguration, name string) error {
	defer perf.Track(atmosConfig, "devcontainer.ShowConfig")()

	config, settings, err := m.configLoader.LoadConfig(atmosConfig, name)
	if err != nil {
		return err
	}

	printSettings(settings)
	printBasicInfo(config)
	printBuildInfo(config)
	printWorkspaceInfo(config)
	printMounts(config)
	printPorts(config)
	printEnv(config)
	printRunArgs(config)
	printRemoteUser(config)

	return nil
}

// printSettings prints runtime settings.
func printSettings(settings *Settings) {
	if settings.Runtime != "" {
		fmt.Printf("Runtime: %s\n\n", settings.Runtime)
	}
}

// printBasicInfo prints basic devcontainer information.
func printBasicInfo(config *Config) {
	fmt.Printf("Name: %s\n", config.Name)
	fmt.Printf("Image: %s\n", config.Image)
}

// printBuildInfo prints build configuration.
func printBuildInfo(config *Config) {
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

// printWorkspaceInfo prints workspace configuration.
func printWorkspaceInfo(config *Config) {
	if config.WorkspaceFolder != "" {
		fmt.Printf("\nWorkspace Folder: %s\n", config.WorkspaceFolder)
	}

	if config.WorkspaceMount != "" {
		fmt.Printf("Workspace Mount: %s\n", config.WorkspaceMount)
	}
}

// printMounts prints mount configurations.
func printMounts(config *Config) {
	if len(config.Mounts) == 0 {
		return
	}

	fmt.Println("\nMounts:")
	for _, mount := range config.Mounts {
		fmt.Printf("  - %s\n", mount)
	}
}

// containsRandomFunction checks if any port uses the !random YAML function.
func containsRandomFunction(forwardPorts []interface{}) bool {
	for _, port := range forwardPorts {
		if str, ok := port.(string); ok {
			if strings.Contains(str, "!random") {
				return true
			}
		}
	}
	return false
}

// printPorts prints port forwarding configuration.
func printPorts(config *Config) {
	ports, err := ParsePorts(config.ForwardPorts, config.PortsAttributes)
	if err != nil {
		_ = ui.Warningf("Error parsing ports: %v", err)
		return
	}
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

	// Warn about !random values.
	if containsRandomFunction(config.ForwardPorts) {
		_ = ui.Warning("Ports using !random will generate new values each time Atmos processes the configuration.\nTo see actual runtime ports, use: atmos devcontainer list")
	}
}

// printEnv prints environment variables.
func printEnv(config *Config) {
	if len(config.ContainerEnv) == 0 {
		return
	}

	fmt.Println("\nEnvironment Variables:")
	for k, v := range config.ContainerEnv {
		fmt.Printf("  %s: %s\n", k, v)
	}
}

// printRunArgs prints container run arguments.
func printRunArgs(config *Config) {
	if len(config.RunArgs) == 0 {
		return
	}

	fmt.Println("\nRun Arguments:")
	for _, arg := range config.RunArgs {
		fmt.Printf("  - %s\n", arg)
	}
}

// printRemoteUser prints remote user configuration.
func printRemoteUser(config *Config) {
	if config.RemoteUser != "" {
		fmt.Printf("\nRemote User: %s\n", config.RemoteUser)
	}
}
