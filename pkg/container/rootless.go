package container

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RuntimeIsRootless reports whether the given runtime runs containers rootlessly
// (in a user namespace). Rootless runtimes — notably the default macOS Podman
// machine — don't delegate every cgroup-v2 controller to containers and run them
// in a user namespace, which some workloads (e.g. k3s) must accommodate.
//
// It is best-effort: on any detection failure it returns false (assume rootful),
// so the default behavior is always the rootful path.
func RuntimeIsRootless(ctx context.Context, runtime Runtime) bool {
	defer perf.Track(nil, "container.RuntimeIsRootless")()

	switch r := runtime.(type) {
	case *PodmanRuntime:
		out, err := r.command(ctx, "info", "--format", "{{.Host.Security.Rootless}}").CombinedOutput()
		return err == nil && strings.TrimSpace(string(out)) == "true"
	case *DockerRuntime:
		// Docker is rootful by default; rootless Docker advertises it in SecurityOptions.
		out, err := r.command(ctx, "info", "--format", "{{.SecurityOptions}}").CombinedOutput()
		return err == nil && strings.Contains(string(out), "rootless")
	default:
		return false
	}
}
