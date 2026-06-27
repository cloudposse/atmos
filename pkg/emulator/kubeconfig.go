package emulator

import (
	"bytes"
	"context"
	"fmt"
	"regexp"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
)

// k3sKubeconfigPath is where k3s writes its admin kubeconfig inside the container.
const k3sKubeconfigPath = "/etc/rancher/k3s/k3s.yaml"

// kubeconfigServerPattern matches the `server:` line in a kubeconfig.
var kubeconfigServerPattern = regexp.MustCompile(`(?m)^(\s*server:\s*).*$`)

// Kubeconfig harvests the admin kubeconfig from a running kubernetes-target
// emulator (k3s) and rewrites its server URL to the live host port so it works
// from the host. The embedded CA and client credentials are preserved verbatim —
// that kubeconfig IS the credential.
func (m *Manager) Kubeconfig(ctx context.Context, stack, name string) ([]byte, error) {
	defer perf.Track(nil, "emulator.Manager.Kubeconfig")()

	runtime, info, err := m.find(ctx, stack, name)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := runtime.Exec(ctx, info.ID, []string{"cat", k3sKubeconfigPath}, &container.ExecOptions{
		AttachStdout: true,
		Stdout:       &buf,
	}); err != nil {
		return nil, fmt.Errorf("%w: read kubeconfig from %s/emulator/%s: %w", errUtils.ErrEmulatorConfigInvalid, stack, name, err)
	}
	hostPort := firstBoundPort(info)
	if hostPort == 0 {
		return nil, fmt.Errorf("%w: %s/emulator/%s has no bound port", errUtils.ErrEmulatorNotRunning, stack, name)
	}
	server := fmt.Sprintf("https://localhost:%d", hostPort)
	return kubeconfigServerPattern.ReplaceAll(buf.Bytes(), []byte("${1}"+server)), nil
}

// firstBoundPort returns the live host port of the first bound container port.
func firstBoundPort(info *container.Info) int {
	for _, binding := range info.Ports {
		if binding.HostPort != 0 {
			return binding.HostPort
		}
	}
	return 0
}
