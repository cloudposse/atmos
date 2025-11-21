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

// RuntimeStatus represents the availability status of a container runtime.
type RuntimeStatus int

const (
	// RuntimeAvailable indicates the runtime is available and responsive.
	RuntimeAvailable RuntimeStatus = iota
	// RuntimeUnavailable indicates the runtime binary is not found.
	RuntimeUnavailable
	// RuntimeNotResponding indicates the runtime binary exists but is not responding.
	RuntimeNotResponding
	// RuntimeNeedsInit indicates Podman is present but needs machine initialization.
	RuntimeNeedsInit
	// RuntimeNeedsStart indicates Podman machine exists but needs to be started.
	RuntimeNeedsStart
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
// This does NOT auto-start Podman machines - use checkRuntimeStatus for detailed status.
func isAvailable(ctx context.Context, runtimeType Type) bool {
	defer perf.Track(nil, "container.isAvailable")()

	status := checkRuntimeStatus(ctx, runtimeType)
	return status == RuntimeAvailable
}

// checkRuntimeStatus returns the detailed availability status of a container runtime.
// This allows callers to distinguish between unavailable, not responding, needs init, etc.
func checkRuntimeStatus(ctx context.Context, runtimeType Type) RuntimeStatus {
	defer perf.Track(nil, "container.checkRuntimeStatus")()

	// Check if binary exists in PATH.
	_, err := globalExecutor.LookPath(string(runtimeType))
	if err != nil {
		log.Debug("Runtime binary not found in PATH", logKeyRuntime, runtimeType)
		return RuntimeUnavailable
	}

	// Check if runtime is responsive.
	cmd := globalExecutor.CommandContext(ctx, string(runtimeType), "info")
	if err := cmd.Run(); err != nil {
		log.Debug("Runtime is not responsive", logKeyRuntime, runtimeType, "error", err)
		return diagnoseUnresponsiveRuntime(ctx, runtimeType)
	}

	return RuntimeAvailable
}

// diagnoseUnresponsiveRuntime determines why a runtime is not responding.
// For Podman, checks if machine needs init or start. For others, returns NotResponding.
func diagnoseUnresponsiveRuntime(ctx context.Context, runtimeType Type) RuntimeStatus {
	// Only Podman has machine-based initialization.
	if runtimeType != TypePodman {
		return RuntimeNotResponding
	}

	// Check if any Podman machine exists.
	if !podmanMachineExists(ctx) {
		log.Debug("Podman machine does not exist - needs initialization", logKeyRuntime, runtimeType)
		return RuntimeNeedsInit
	}

	// Machine exists but not running.
	log.Debug("Podman machine exists but not running - needs start", logKeyRuntime, runtimeType)
	return RuntimeNeedsStart
}

// TryRecoverPodmanRuntime attempts to recover Podman by initializing/starting the machine.
// This is an opt-in operation that should only be called when the user explicitly requests it.
// Returns the new status after recovery attempt.
func TryRecoverPodmanRuntime(ctx context.Context) RuntimeStatus {
	defer perf.Track(nil, "container.TryRecoverPodmanRuntime")()

	status := checkRuntimeStatus(ctx, TypePodman)

	switch status {
	case RuntimeNeedsInit:
		// Initialize and start the machine.
		if err := initializePodmanMachine(ctx); err != nil {
			log.Debug("Failed to initialize Podman machine", "error", err)
			return RuntimeNeedsInit
		}
		if err := startPodmanMachine(ctx); err != nil {
			log.Debug("Failed to start Podman machine after init", "error", err)
			return RuntimeNeedsStart
		}

	case RuntimeNeedsStart:
		// Just start the existing machine.
		if err := startPodmanMachine(ctx); err != nil {
			log.Debug("Failed to start Podman machine", "error", err)
			return RuntimeNeedsStart
		}

	default:
		return status
	}

	// Verify recovery succeeded.
	cmd := globalExecutor.CommandContext(ctx, string(TypePodman), "info")
	if err := cmd.Run(); err != nil {
		log.Debug("Podman still not responsive after recovery attempt", "error", err)
		return RuntimeNotResponding
	}

	log.Debug("Successfully recovered Podman runtime")
	return RuntimeAvailable
}

// RuntimeStatusMessage returns a user-friendly message describing the runtime status.
func RuntimeStatusMessage(status RuntimeStatus, runtimeType Type) string {
	defer perf.Track(nil, "container.RuntimeStatusMessage")()

	switch status {
	case RuntimeAvailable:
		return fmt.Sprintf("%s is available and running", runtimeType)
	case RuntimeUnavailable:
		return fmt.Sprintf("%s is not installed or not in PATH", runtimeType)
	case RuntimeNotResponding:
		return fmt.Sprintf("%s is installed but not responding", runtimeType)
	case RuntimeNeedsInit:
		return "Podman is installed but no machine exists. Run 'podman machine init' to create one."
	case RuntimeNeedsStart:
		return "Podman machine exists but is not running. Run 'podman machine start' to start it."
	default:
		return fmt.Sprintf("unknown status for %s", runtimeType)
	}
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
