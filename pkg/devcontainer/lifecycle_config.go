package devcontainer

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/data"
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
		_ = data.Writef("Runtime: %s\n\n", settings.Runtime)
	}
}

// printBasicInfo prints basic devcontainer information.
func printBasicInfo(config *Config) {
	_ = data.Writef("Name: %s\n", config.Name)
	_ = data.Writef("Image: %s\n", config.Image)
}

// printBuildInfo prints build configuration.
func printBuildInfo(config *Config) {
	if config.Build == nil {
		return
	}

	_ = data.Writeln("\nBuild:")
	_ = data.Writef("  Dockerfile: %s\n", config.Build.Dockerfile)
	_ = data.Writef("  Context: %s\n", config.Build.Context)

	if len(config.Build.Args) > 0 {
		_ = data.Writeln("  Args:")
		for k, v := range config.Build.Args {
			_ = data.Writef("    %s: %s\n", k, v)
		}
	}
}

// printWorkspaceInfo prints workspace configuration.
func printWorkspaceInfo(config *Config) {
	if config.WorkspaceFolder != "" {
		_ = data.Writef("\nWorkspace Folder: %s\n", config.WorkspaceFolder)
	}

	if config.WorkspaceMount != "" {
		_ = data.Writef("Workspace Mount: %s\n", config.WorkspaceMount)
	}
}

// printMounts prints mount configurations.
func printMounts(config *Config) {
	if len(config.Mounts) == 0 {
		return
	}

	_ = data.Writeln("\nMounts:")
	for _, mount := range config.Mounts {
		_ = data.Writef("  - %s\n", mount)
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
		ui.Warningf("Error parsing ports: %v", err)
		return
	}
	if len(ports) == 0 {
		return
	}

	_ = data.Writeln("\nForward Ports:")
	for _, port := range ports {
		if port.HostPort == port.ContainerPort {
			_ = data.Writef("  - %d\n", port.ContainerPort)
		} else {
			_ = data.Writef("  - %d:%d\n", port.HostPort, port.ContainerPort)
		}
	}

	// Warn about !random values.
	if containsRandomFunction(config.ForwardPorts) {
		ui.Warning("Ports using !random will generate new values each time Atmos processes the configuration.\nTo see actual runtime ports, use: atmos devcontainer list")
	}
}

// printEnv prints environment variables.
func printEnv(config *Config) {
	if len(config.ContainerEnv) == 0 {
		return
	}

	_ = data.Writeln("\nEnvironment Variables:")
	for k, v := range config.ContainerEnv {
		_ = data.Writef("  %s: %s\n", k, v)
	}
}

// printRunArgs prints container run arguments.
func printRunArgs(config *Config) {
	if len(config.RunArgs) == 0 {
		return
	}

	_ = data.Writeln("\nRun Arguments:")
	for _, arg := range config.RunArgs {
		_ = data.Writef("  - %s\n", arg)
	}
}

// printRemoteUser prints remote user configuration.
func printRemoteUser(config *Config) {
	if config.RemoteUser != "" {
		_ = data.Writef("\nRemote User: %s\n", config.RemoteUser)
	}
}
