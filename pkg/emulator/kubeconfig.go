package emulator

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
)

// k3sKubeconfigPath is where k3s writes its admin kubeconfig inside the container.
const k3sKubeconfigPath = "/etc/rancher/k3s/k3s.yaml"

// kubeconfigReadyTimeout bounds how long Kubeconfig waits for k3s to write its admin
// kubeconfig. `emulator up` returns once the API port is reachable, but k3s writes
// /etc/rancher/k3s/k3s.yaml a moment later, so a one-shot read races. These are vars
// (not consts) so tests can shrink them to keep the poll loop fast.
var (
	kubeconfigReadyTimeout = 90 * time.Second
	// Poll interval while waiting for the kubeconfig to be written.
	kubeconfigPollInterval = time.Second
)

// kubeconfigServerPattern matches the `server:` line in a kubeconfig.
var kubeconfigServerPattern = regexp.MustCompile(`(?m)^(\s*server:\s*).*$`)

// Kubeconfig harvests the admin kubeconfig from a running kubernetes-target
// emulator (k3s) and rewrites its server URL to the live host port so it works
// from the host. The embedded CA and client credentials are preserved verbatim —
// that kubeconfig IS the credential.
//
// It polls until the kubeconfig is readable (k3s writes it shortly after the API
// port opens) or the timeout elapses, so callers invoked immediately after
// `emulator up` do not lose the readiness race.
func (m *Manager) Kubeconfig(ctx context.Context, stack, name string) ([]byte, error) {
	defer perf.Track(nil, "emulator.Manager.Kubeconfig")()

	deadline := time.Now().Add(kubeconfigReadyTimeout)
	var lastErr error
	for {
		kubeconfig, retryable, err := m.harvestKubeconfig(ctx, stack, name)
		if err == nil {
			return kubeconfig, nil
		}
		lastErr = err

		// Only the readiness race (container up, kubeconfig not yet written) is worth
		// polling. A missing container or an unbound port is terminal — fail fast.
		// Use >= (reached-or-passed) rather than strictly-after so a zero timeout makes
		// exactly one attempt on every platform: on coarse-granularity clocks (Windows)
		// `time.Now()` can still equal the deadline after the first attempt, and a
		// strict After() would spuriously poll again.
		if !retryable || !time.Now().Before(deadline) {
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, lastErr
		case <-time.After(kubeconfigPollInterval):
		}
	}
}

// harvestKubeconfig performs a single attempt to read and rewrite the admin kubeconfig
// from the running k3s emulator container. The retryable return reports whether the
// failure is a transient readiness race (k3s has not written k3s.yaml yet) worth
// polling, versus a terminal condition (emulator not running, no bound port).
func (m *Manager) harvestKubeconfig(ctx context.Context, stack, name string) (kubeconfig []byte, retryable bool, err error) {
	runtime, info, err := m.find(ctx, stack, name)
	if err != nil {
		// Emulator not running (or lookup failed) — terminal, do not poll.
		return nil, false, err
	}
	var buf bytes.Buffer
	if execErr := runtime.Exec(ctx, info.ID, []string{"cat", k3sKubeconfigPath}, &container.ExecOptions{
		AttachStdout: true,
		Stdout:       &buf,
	}); execErr != nil {
		// k3s writes /etc/rancher/k3s/k3s.yaml shortly after the API port opens, so a
		// failed read right after `up` is usually the readiness race — worth retrying.
		return nil, true, fmt.Errorf("%w: read kubeconfig from %s/emulator/%s: %w", errUtils.ErrEmulatorConfigInvalid, stack, name, execErr)
	}
	if buf.Len() == 0 {
		return nil, true, fmt.Errorf("%w: kubeconfig from %s/emulator/%s is not ready yet", errUtils.ErrEmulatorNotRunning, stack, name)
	}
	hostPort := firstBoundPort(info)
	if hostPort == 0 {
		return nil, false, fmt.Errorf("%w: %s/emulator/%s has no bound port", errUtils.ErrEmulatorNotRunning, stack, name)
	}
	server := fmt.Sprintf("https://localhost:%d", hostPort)
	return kubeconfigServerPattern.ReplaceAll(buf.Bytes(), []byte("${1}"+server)), false, nil
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
