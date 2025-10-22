package devcontainer

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DetectRuntime detects the container runtime based on settings.
// If runtimeSetting is specified ("docker" or "podman"), it uses that.
// Otherwise, it auto-detects the runtime.
func DetectRuntime(runtimeSetting string) (container.Runtime, error) {
	defer perf.Track(nil, "devcontainer.DetectRuntime")()

	ctx := context.Background()

	// If runtime is explicitly specified in settings, use it.
	if runtimeSetting != "" {
		switch runtimeSetting {
		case "docker":
			os.Setenv("ATMOS_CONTAINER_RUNTIME", "docker")
		case "podman":
			os.Setenv("ATMOS_CONTAINER_RUNTIME", "podman")
		default:
			return nil, fmt.Errorf("%w: invalid runtime setting '%s', must be 'docker' or 'podman'", errUtils.ErrInvalidDevcontainerConfig, runtimeSetting)
		}
	}

	return container.DetectRuntime(ctx)
}

// ToCreateConfig converts a devcontainer config to container.CreateConfig.
func ToCreateConfig(config *Config, containerName, devcontainerName, instance string) *container.CreateConfig {
	defer perf.Track(nil, "devcontainer.ToCreateConfig")()

	cwd := getCurrentWorkingDirectory()

	createConfig := &container.CreateConfig{
		Name:            containerName,
		Image:           config.Image,
		WorkspaceFolder: config.WorkspaceFolder,
		Env:             config.ContainerEnv,
		User:            config.RemoteUser,
		Labels:          createDevcontainerLabels(devcontainerName, instance, cwd),
		RunArgs:         config.RunArgs,
		OverrideCommand: config.OverrideCommand,
		Init:            config.Init,
		Privileged:      config.Privileged,
		CapAdd:          config.CapAdd,
		SecurityOpt:     config.SecurityOpt,
	}

	createConfig.Mounts = convertMounts(config, cwd)
	createConfig.Ports = convertPorts(config.ForwardPorts)

	return createConfig
}

func getCurrentWorkingDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func createDevcontainerLabels(devcontainerName, instance, cwd string) map[string]string {
	labels := map[string]string{
		LabelType:                 "devcontainer",
		LabelDevcontainerName:     devcontainerName,
		LabelDevcontainerInstance: instance,
		LabelCreated:              time.Now().Format(time.RFC3339),
	}

	if cwd != "" && cwd != "." {
		labels[LabelWorkspace] = cwd
	}

	return labels
}

func convertMounts(config *Config, cwd string) []container.Mount {
	mounts := make([]container.Mount, 0, len(config.Mounts)+1)

	for _, mountStr := range config.Mounts {
		expandedMountStr := expandDevcontainerVars(mountStr, cwd)
		if mount := parseMountString(expandedMountStr); mount != nil {
			mounts = append(mounts, *mount)
		}
	}

	if config.WorkspaceMount != "" {
		expandedMountStr := expandDevcontainerVars(config.WorkspaceMount, cwd)
		if mount := parseMountString(expandedMountStr); mount != nil {
			mounts = append(mounts, *mount)
		}
	}

	return mounts
}

func convertPorts(forwardPorts []any) []container.PortBinding {
	if len(forwardPorts) == 0 {
		return nil
	}

	ports := make([]container.PortBinding, 0, len(forwardPorts))

	for _, port := range forwardPorts {
		portNum := parsePortNumber(port)
		if portNum > 0 {
			ports = append(ports, container.PortBinding{
				ContainerPort: portNum,
				HostPort:      portNum,
				Protocol:      "tcp",
			})
		}
	}

	return ports
}

func parsePortNumber(port any) int {
	switch v := port.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		var portNum int
		_, _ = fmt.Sscanf(v, "%d", &portNum)
		return portNum
	default:
		return 0
	}
}

// parseMountString parses a devcontainer mount string into a container.Mount.
// Format: "type=bind,source=/foo,target=/bar,readonly"
// Returns nil if parsing fails.
func parseMountString(mountStr string) *container.Mount {
	defer perf.Track(nil, "devcontainer.parseMountString")()

	mount := &container.Mount{}
	parts := strings.Split(mountStr, ",")

	for _, part := range parts {
		parseMountPart(part, mount)
	}

	// Validate required fields.
	if mount.Type == "" || mount.Target == "" {
		return nil
	}

	return mount
}

// parseMountPart parses a single part of a mount string and updates the mount.
func parseMountPart(part string, mount *container.Mount) {
	kv := strings.SplitN(part, "=", 2)
	if len(kv) == 1 {
		// Flag without value (like "readonly").
		key := strings.TrimSpace(kv[0])
		if key == "readonly" {
			mount.ReadOnly = true
		}
		return
	}

	key := strings.TrimSpace(kv[0])
	value := strings.TrimSpace(kv[1])

	switch key {
	case "type":
		mount.Type = value
	case "source", "src":
		mount.Source = value
	case "target", "dst", "destination":
		mount.Target = value
	case "readonly", "ro":
		mount.ReadOnly = value == "true" || value == "1"
	}
}

// expandDevcontainerVars expands devcontainer variables in a string.
// Supported variables:
// - ${localWorkspaceFolder} - Current working directory.
// - ${localEnv:VAR} - Environment variable VAR.
func expandDevcontainerVars(s, workspaceFolder string) string {
	defer perf.Track(nil, "devcontainer.expandDevcontainerVars")()

	// Replace ${localWorkspaceFolder}.
	s = strings.ReplaceAll(s, "${localWorkspaceFolder}", workspaceFolder)

	// Replace ${localEnv:VAR} patterns.
	// Find all ${localEnv:VAR} occurrences.
	for {
		start := strings.Index(s, "${localEnv:")
		if start == -1 {
			break
		}

		end := strings.Index(s[start:], "}")
		if end == -1 {
			break
		}
		end += start

		// Extract variable name.
		varName := s[start+len("${localEnv:") : end]
		_ = viper.BindEnv(varName, varName)
		envValue := viper.GetString(varName)

		// Replace the entire ${localEnv:VAR} with the value.
		s = s[:start] + envValue + s[end+1:]
	}

	return s
}
