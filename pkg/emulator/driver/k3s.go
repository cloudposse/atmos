package driver

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/emulator/target"
)

// k3s is a lightweight, single-binary Kubernetes distribution that runs as one
// privileged container (rancher/k3s) exposing the API on 6443 — a clean fit for
// the single-container emulator lifecycle. Its connection profile is a kubeconfig
// harvested from the running container by the kubernetes/emulator identity, so the
// driver only supplies image/port defaults (KubernetesProfile is a placeholder).
//
// The k3d and kind tools orchestrate Docker themselves (multiple containers via
// their own CLIs) rather than running as a single container, so they do not fit
// this lifecycle and are deferred.
const (
	k3sImage = "rancher/k3s:latest"
	k3sPort  = 6443

	// The k3sRootlessScript is the entrypoint used under a ROOTLESS runtime. Rootless
	// containers run in a user namespace and (by default) don't get every cgroup-v2
	// controller, which breaks the kubelet. This shim — the same pattern kind/k3d
	// use — moves PID 1 into an `init` sub-cgroup and enables the available
	// controllers for child cgroups (satisfying cgroup-v2's no-internal-processes
	// rule), then starts the server with the kubelet's user-namespace feature flag.
	//
	// PREREQUISITE: the host must delegate the `cpuset` (and `cpu`) cgroup-v2
	// controllers to the rootless user (see the demo-helmfile README). The shim can
	// only enable controllers that were delegated to the container.
	k3sRootlessScript = "mkdir -p /sys/fs/cgroup/init; " +
		"echo 1 > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true; " +
		"for c in $(cat /sys/fs/cgroup/cgroup.controllers); do " +
		"echo +$c > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true; done; " +
		"exec /bin/k3s server --kubelet-arg=feature-gates=KubeletInUserNamespace=true"
)

func init() {
	// k3s runs a nested Kubernetes and only works in a privileged container. The
	// server also requires a node token (env K3S_TOKEN) to start. ROOTFUL is the
	// default (`server`); under a rootless runtime the manager swaps in the
	// cgroup-nesting entrypoint via the RootlessOverride below.
	emu.RegisterDriver(&builtinDriver{
		name:            "k3s",
		target:          emu.TargetKubernetes,
		image:           k3sImage,
		ports:           []int{k3sPort},
		privileged:      true,
		env:             map[string]string{"K3S_TOKEN": "atmos-emulator"},
		command:         []string{"server"},
		rootlessRunArgs: []string{"--entrypoint", "/bin/sh"},
		rootlessCommand: []string{"-c", k3sRootlessScript},
		// No default health check: the kubernetes/emulator identity waits for the
		// API while harvesting the kubeconfig, and a privileged k3s boot is too
		// environment-sensitive to gate `up` on by default. Users can still set
		// `container.healthcheck` explicitly.
		restart: defaultEmulatorRestart,
		profile: target.KubernetesProfile,
	})
}
