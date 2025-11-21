package container

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
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
	_, err := globalExecutor.LookPath(string(runtimeType))
	if err != nil {
		log.Debug("Runtime binary not found in PATH", logKeyRuntime, runtimeType)
		return false
	}

	// Check if runtime is responsive.
	cmd := globalExecutor.CommandContext(ctx, string(runtimeType), "info")
	if err := cmd.Run(); err != nil {
		log.Debug("Runtime is not responsive", logKeyRuntime, runtimeType, "error", err)
		return tryRecoverRuntime(ctx, runtimeType)
	}

	return true
}

// tryRecoverRuntime attempts to recover an unresponsive runtime.
// For Podman, tries to start the machine. Returns true if runtime is recovered.
func tryRecoverRuntime(ctx context.Context, runtimeType Type) bool {
	// For Podman, try to auto-start the machine.
	if runtimeType != TypePodman {
		return false
	}

	if !tryStartPodmanMachine(ctx) {
		return false
	}

	// Retry check after starting machine.
	cmd := globalExecutor.CommandContext(ctx, string(runtimeType), "info")
	if err := cmd.Run(); err == nil {
		log.Debug("Successfully started Podman machine", logKeyRuntime, runtimeType)
		return true
	}

	return false
}

// tryStartPodmanMachine attempts to start or initialize the default Podman machine.
// Returns true if machine is ready, false otherwise.
func tryStartPodmanMachine(ctx context.Context) bool {
	defer perf.Track(nil, "container.tryStartPodmanMachine")()

	// Check if any machine exists.
	if !podmanMachineExists(ctx) {
		// No machine exists - initialize one first.
		if err := initializePodmanMachine(ctx); err != nil {
			log.Debug("Failed to initialize Podman machine", "error", err)
			return false
		}
	}

	// Start the machine.
	if err := startPodmanMachine(ctx); err != nil {
		log.Debug("Failed to start Podman machine", "error", err)
		return false
	}

	return true
}

// podmanMachineExists checks if any Podman machine exists.
func podmanMachineExists(ctx context.Context) bool {
	defer perf.Track(nil, "container.podmanMachineExists")()

	cmd := globalExecutor.CommandContext(ctx, "podman", "machine", "list", "--format", "{{.Name}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Debug("Failed to list Podman machines", "error", err)
		return false
	}

	return hasMachines(string(output))
}

// hasMachines checks if the output from 'podman machine list' contains any machine names.
// Returns true if there's any non-whitespace content, false otherwise.
func hasMachines(output string) bool {
	machines := strings.TrimSpace(output)
	return machines != ""
}

// initializePodmanMachine initializes a new Podman machine with spinner UI.
func initializePodmanMachine(ctx context.Context) error {
	defer perf.Track(nil, "container.initializePodmanMachine")()

	return spinner.ExecWithSpinner(
		"Initializing Podman machine",
		"Initialized Podman machine",
		func() error {
			cmd := globalExecutor.CommandContext(ctx, "podman", "machine", "init")
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to initialize: %w: %s", err, string(output))
			}
			return nil
		})
}

// startPodmanMachine starts the Podman machine with spinner UI.
func startPodmanMachine(ctx context.Context) error {
	defer perf.Track(nil, "container.startPodmanMachine")()

	return spinner.ExecWithSpinner(
		"Starting Podman machine",
		"Started Podman machine",
		func() error {
			cmd := globalExecutor.CommandContext(ctx, "podman", "machine", "start")
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to start: %w: %s", err, string(output))
			}
			return nil
		})
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
