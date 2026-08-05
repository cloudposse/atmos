package emulator

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// K3sKubeconfigPath is where k3s writes its admin kubeconfig inside the container.
	k3sKubeconfigPath = "/etc/rancher/k3s/k3s.yaml"
	// K3sAPIServerPort is the container port for the Kubernetes API server.
	k3sAPIServerPort = 6443
)

// kubeconfigReadyTimeout bounds how long Kubeconfig waits for k3s to write its admin
// kubeconfig. `emulator up` returns once the API port is reachable, but k3s writes
// /etc/rancher/k3s/k3s.yaml a moment later, so a one-shot read races. These are vars
// (not consts) so tests can shrink them to keep the poll loop fast.
var (
	kubeconfigReadyTimeout = 90 * time.Second
	// Poll interval while waiting for the kubeconfig to be written.
	kubeconfigPollInterval = time.Second
	// KubernetesReadyTimeout bounds how long `emulator up` waits for k3s to
	// register a Ready node after kubeconfig is harvestable. MacOS virtualized
	// Docker runtimes can take several minutes to settle a fresh k3s node.
	kubernetesReadyTimeout = 5 * time.Minute
	// Poll interval while waiting for the k3s node to become Ready.
	kubernetesReadyPollInterval = 2 * time.Second
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
		// Bound each attempt by the earlier of the caller deadline and the readiness deadline,
		// so a stalled runtime.List/runtime.Exec cannot block past kubeconfigReadyTimeout.
		attemptDeadline := deadline
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(attemptDeadline) {
			attemptDeadline = ctxDeadline
		}
		attemptCtx, cancel := context.WithDeadline(ctx, attemptDeadline)
		kubeconfig, retryable, err := m.harvestKubeconfig(attemptCtx, stack, name)
		cancel()
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
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !retryable || !time.Now().Before(deadline) {
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
	hostPort := apiServerHostPort(info)
	if hostPort == 0 {
		return nil, false, fmt.Errorf("%w: %s/emulator/%s has no bound API server port", errUtils.ErrEmulatorNotRunning, stack, name)
	}
	// Use the IPv4 loopback literal, not "localhost": on Linux the runtime
	// publishes the API-server port on IPv4 only, while "localhost" resolves to
	// IPv6 ::1 first, and a connect to ::1 against an IPv4-only published port
	// hangs rather than refusing (see loopbackHostToIPv4 in profile.go). k3s's
	// serving certificate includes 127.0.0.1 in its SANs, so TLS still verifies.
	server := fmt.Sprintf("https://%s:%d", loopbackHostToIPv4("localhost"), hostPort)
	return kubeconfigServerPattern.ReplaceAll(buf.Bytes(), []byte("${1}"+server)), false, nil
}

// apiServerHostPort returns the live host port bound to the k3s API server.
func apiServerHostPort(info *container.Info) int {
	for _, binding := range info.Ports {
		if binding.ContainerPort == k3sAPIServerPort && binding.HostPort != 0 {
			return binding.HostPort
		}
	}
	return 0
}

// waitKubernetesReady waits for k3s to register at least one Ready node. A
// harvestable kubeconfig is necessary but not sufficient: on slower runtimes the
// API can still reject immediate Helm release lookups while controllers settle.
func (m *Manager) waitKubernetesReady(ctx context.Context, runtime container.Runtime, stack, name string) error {
	deadline := time.Now().Add(kubernetesReadyTimeout)
	var lastErr error
	for {
		ready, err := m.kubernetesNodeReady(ctx, runtime, stack, name)
		if err == nil && ready {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("%w: %s/emulator/%s Kubernetes node is not ready yet", errUtils.ErrEmulatorNotRunning, stack, name)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !time.Now().Before(deadline) {
			return lastErr
		}
		wait := time.Until(deadline)
		if wait > kubernetesReadyPollInterval {
			wait = kubernetesReadyPollInterval
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *Manager) kubernetesNodeReady(ctx context.Context, runtime container.Runtime, stack, name string) (bool, error) {
	info, found, err := container.FindInstance(ctx, runtime, stack, cfg.EmulatorComponentType, name)
	if err != nil {
		return false, err
	}
	if !found || !container.IsContainerRunning(info.Status) {
		return false, fmt.Errorf("%w: %s/emulator/%s is not running", errUtils.ErrEmulatorNotRunning, stack, name)
	}
	var buf bytes.Buffer
	if err := runtime.Exec(ctx, info.ID, []string{"kubectl", "get", "nodes", "--no-headers"}, &container.ExecOptions{
		AttachStdout: true,
		Stdout:       &buf,
	}); err != nil {
		return false, err
	}
	return kubernetesNodeListHasReadyNode(buf.String()), nil
}

func kubernetesNodeListHasReadyNode(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		for _, condition := range strings.Split(fields[1], ",") {
			if condition == "Ready" {
				return true
			}
		}
	}
	return false
}
