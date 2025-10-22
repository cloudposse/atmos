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
		log.Debug("Using container runtime from ATMOS_CONTAINER_RUNTIME", "runtime", envRuntime)

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
		log.Debug("Auto-detected container runtime", "runtime", "docker")
		return NewDockerRuntime(), nil
	}

	// Try Podman
	if isAvailable(ctx, TypePodman) {
		log.Debug("Auto-detected container runtime", "runtime", "podman")
		return NewPodmanRuntime(), nil
	}

	return nil, fmt.Errorf("%w: neither docker nor podman is available", errUtils.ErrRuntimeNotAvailable)
}

// isAvailable checks if a container runtime is available and running.
func isAvailable(ctx context.Context, runtimeType Type) bool {
	defer perf.Track(nil, "container.isAvailable")()

	// Check if binary exists in PATH
	_, err := exec.LookPath(string(runtimeType))
	if err != nil {
		log.Debug("Runtime binary not found in PATH", "runtime", runtimeType)
		return false
	}

	// Check if runtime is responsive.
	cmd := exec.CommandContext(ctx, string(runtimeType), "info") //nolint:gosec // runtimeType is from enum, not user input
	if err := cmd.Run(); err != nil {
		log.Debug("Runtime is not responsive", "runtime", runtimeType, "error", err)
		return false
	}

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
