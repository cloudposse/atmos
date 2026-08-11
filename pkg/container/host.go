package container

import (
	"context"
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// hostRuntimeSocketTarget is where the host runtime socket is mounted inside the
// container — the de-facto path tools (including MiniStack) expect.
const hostRuntimeSocketTarget = "/var/run/docker.sock"

// defaultDockerSocket is the conventional Docker socket path used when DOCKER_HOST
// does not name a unix socket.
const defaultDockerSocket = "/var/run/docker.sock"

// prepareHostRuntime grants a container access to the host container runtime
// (Docker-out-of-Docker) when config.Host is set: it warns on rootless podman (where
// the socket is unreachable in-container), resolves the host runtime socket, and
// mutates the CreateConfig to mount it, run as root, relabel for SELinux, and set
// DOCKER_HOST. Called from the runtime Create chokepoint that every surface reaches.
func prepareHostRuntime(ctx context.Context, runtime Runtime, config *CreateConfig) error {
	defer perf.Track(nil, "container.prepareHostRuntime")()

	if config == nil || !config.Host {
		return nil
	}
	warnIfRootlessPodman(ctx, runtime)
	socket, err := HostRuntimeSocket(ctx, runtime)
	if err != nil {
		return err
	}
	applyHostRuntime(config, socket)
	return nil
}

// HostRuntimeSocket returns the host container runtime socket path for the active
// runtime, used to grant a container access to that runtime (Docker-out-of-Docker).
func HostRuntimeSocket(ctx context.Context, runtime Runtime) (string, error) {
	defer perf.Track(nil, "container.HostRuntimeSocket")()

	switch r := runtime.(type) {
	case *PodmanRuntime:
		out, err := r.command(ctx, "info", "--format", "{{.Host.RemoteSocket.Path}}").CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("%w: resolve podman socket: %w", errUtils.ErrContainerRuntimeOperation, err)
		}
		socket := strings.TrimSpace(string(out))
		if socket == "" {
			return "", fmt.Errorf("%w: podman reported no remote socket", errUtils.ErrContainerRuntimeOperation)
		}
		return socket, nil
	case *DockerRuntime:
		return dockerHostSocket(), nil
	default:
		return defaultDockerSocket, nil
	}
}

// dockerHostSocket resolves the Docker socket from DOCKER_HOST (when it is a unix
// socket) or the conventional default.
func dockerHostSocket() string {
	// DOCKER_HOST is Docker's own environment variable (read by every Docker SDK/CLI),
	// not an Atmos config var, so it is read directly rather than via viper.BindEnv.
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") { //nolint:forbidigo // external tool env var, not Atmos config.
		return strings.TrimPrefix(h, "unix://")
	}
	return defaultDockerSocket
}

// applyHostRuntime mutates the CreateConfig to grant the container access to the host
// container runtime: bind-mount the runtime socket, run as root (unless a user is
// already pinned), relabel the mount for SELinux, and advertise the socket via
// DOCKER_HOST so SDKs/CLIs inside the container find it.
func applyHostRuntime(config *CreateConfig, socket string) {
	config.Mounts = append(config.Mounts, Mount{
		Type:   "bind",
		Source: socket,
		Target: hostRuntimeSocketTarget,
	})
	if config.User == "" {
		config.User = "0"
	}
	config.SecurityOpt = append(config.SecurityOpt, "label=disable")
	if config.Env == nil {
		config.Env = map[string]string{}
	}
	if _, ok := config.Env["DOCKER_HOST"]; !ok {
		config.Env["DOCKER_HOST"] = "unix://" + hostRuntimeSocketTarget
	}
}

// warnIfRootlessPodman logs a clear, actionable warning when host-runtime access is
// requested on a rootless podman runtime, where the bind-mounted socket is unreachable
// inside the container (the user-namespace boundary).
func warnIfRootlessPodman(ctx context.Context, runtime Runtime) {
	if _, ok := runtime.(*PodmanRuntime); !ok {
		return
	}
	if RuntimeIsRootless(ctx, runtime) {
		log.Warn("container.runtime.host requested but the active runtime is rootless podman; " +
			"the container cannot reach the host runtime — use Docker or `podman machine set --rootful`")
	}
}
