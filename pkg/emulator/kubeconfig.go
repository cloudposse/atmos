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

var kubeconfigReadyTimeout = 90 * time.Second

var kubeconfigPollInterval = time.Second

// kubeconfigServerPattern matches the `server:` line in a kubeconfig.
var kubeconfigServerPattern = regexp.MustCompile(`(?m)^(\s*server:\s*).*$`)

// Kubeconfig harvests the admin kubeconfig from a running kubernetes-target
// emulator (k3s) and rewrites its server URL to the live host port so it works
// from the host. The embedded CA and client credentials are preserved verbatim —
// that kubeconfig IS the credential.
func (m *Manager) Kubeconfig(ctx context.Context, stack, name string) ([]byte, error) {
	defer perf.Track(nil, "emulator.Manager.Kubeconfig")()

	deadline := time.Now().Add(kubeconfigReadyTimeout)
	var lastErr error
	for {
		// Bound each attempt by the earlier of the caller deadline and the readiness deadline,
		// so a stalled runtime.List/runtime.Exec cannot block past kubeconfigReadyTimeout.
		attemptDeadline := deadline
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(attemptDeadline) {
			attemptDeadline = ctxDeadline
		}
		attemptCtx, cancel := context.WithDeadline(ctx, attemptDeadline)
		kubeconfig, err := m.tryKubeconfig(attemptCtx, stack, name)
		cancel()
		if err == nil {
			return kubeconfig, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		// Sleep for min(remaining time until deadline, pollInterval) to avoid
		// overshooting the deadline by a full poll interval.
		wait := time.Until(deadline)
		if wait > kubeconfigPollInterval {
			wait = kubeconfigPollInterval
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *Manager) tryKubeconfig(ctx context.Context, stack, name string) ([]byte, error) {
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
	// Use the IPv4 loopback literal, not "localhost": on Linux the runtime
	// publishes the API-server port on IPv4 only, while "localhost" resolves to
	// IPv6 ::1 first, and a connect to ::1 against an IPv4-only published port
	// hangs rather than refusing (see loopbackHostToIPv4 in profile.go). k3s's
	// serving certificate includes 127.0.0.1 in its SANs, so TLS still verifies.
	server := fmt.Sprintf("https://%s:%d", loopbackHostToIPv4("localhost"), hostPort)
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
