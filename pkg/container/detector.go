package container

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	logKeyRuntime = "runtime"
)

// DetectRuntime auto-detects the available container runtime.
// Priority order:
// 1. ATMOS_CONTAINER_RUNTIME environment variable
// 2. Docker (if available and running)
// 3. Podman (if available and running)
// Returns error if no runtime is available.
func DetectRuntime(ctx context.Context) (Runtime, error) {
	defer perf.Track(nil, "container.DetectRuntime")()

	// Check environment variable first.
	_ = viper.BindEnv("ATMOS_CONTAINER_RUNTIME", "ATMOS_CONTAINER_RUNTIME")
	if envRuntime := viper.GetString("ATMOS_CONTAINER_RUNTIME"); envRuntime != "" {
		log.Debug("Using container runtime from ATMOS_CONTAINER_RUNTIME", logKeyRuntime, envRuntime)

		switch envRuntime {
		case string(TypeDocker):
			if isAvailable(ctx, TypeDocker) {
				return NewDockerRuntime(), nil
			}
			return nil, fmt.Errorf("%w: docker is not available or not running", errUtils.ErrRuntimeNotAvailable)

		case string(TypePodman):
			if isAvailable(ctx, TypePodman) {
				return NewPodmanRuntime(), nil
			}
			return nil, fmt.Errorf("%w: podman is not available or not running", errUtils.ErrRuntimeNotAvailable)

		default:
			return nil, fmt.Errorf("%w: unknown runtime type '%s'", errUtils.ErrRuntimeNotAvailable, envRuntime)
		}
	}

	// Try Docker first
	if isAvailable(ctx, TypeDocker) {
		log.Debug("Auto-detected container runtime", logKeyRuntime, "docker")
		return NewDockerRuntime(), nil
	}

	// Try Podman
	if isAvailable(ctx, TypePodman) {
		log.Debug("Auto-detected container runtime", logKeyRuntime, "podman")
		return NewPodmanRuntime(), nil
	}

	return nil, fmt.Errorf("%w: neither docker nor podman is available", errUtils.ErrRuntimeNotAvailable)
}

// isAvailable checks if a container runtime is available and running.
// For Podman, attempts to auto-start the machine if not running.
func isAvailable(ctx context.Context, runtimeType Type) bool {
	defer perf.Track(nil, "container.isAvailable")()

	// Check if binary exists in PATH.
	_, err := exec.LookPath(string(runtimeType))
	if err != nil {
		log.Debug("Runtime binary not found in PATH", logKeyRuntime, runtimeType)
		return false
	}

	// Check if runtime is responsive.
	cmd := exec.CommandContext(ctx, string(runtimeType), "info") //nolint:gosec // runtimeType is from enum, not user input
	if err := cmd.Run(); err != nil {
		log.Debug("Runtime is not responsive", logKeyRuntime, runtimeType, "error", err)

		// For Podman, try to auto-start the machine.
		if runtimeType == TypePodman {
			if tryStartPodmanMachine(ctx) {
				// Retry check after starting machine.
				cmd := exec.CommandContext(ctx, string(runtimeType), "info") //nolint:gosec // runtimeType is from enum, not user input
				if err := cmd.Run(); err == nil {
					log.Debug("Successfully started Podman machine", logKeyRuntime, runtimeType)
					return true
				}
			}
		}

		return false
	}

	return true
}

// tryStartPodmanMachine attempts to start the default Podman machine.
// Returns true if machine was started successfully, false otherwise.
func tryStartPodmanMachine(ctx context.Context) bool {
	defer perf.Track(nil, "container.tryStartPodmanMachine")()

	log.Info("Podman machine is not running. Starting Podman machine...")

	// Try to start the machine.
	cmd := exec.CommandContext(ctx, "podman", "machine", "start")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Debug("Failed to start Podman machine", "error", err, "output", string(output))
		return false
	}

	log.Info("Successfully started Podman machine")
	log.Debug("Podman machine start output", "output", string(output))
	return true
}

// GetRuntimeType returns the type of a runtime instance.
func GetRuntimeType(runtime Runtime) Type {
	defer perf.Track(nil, "container.GetRuntimeType")()

	switch runtime.(type) {
	case *DockerRuntime:
		return TypeDocker
	case *PodmanRuntime:
		return TypePodman
	default:
		return ""
	}
}
