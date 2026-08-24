package container

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DefaultHealthyTimeout bounds how long WaitHealthy waits for a container to
	// report a healthy state before giving up.
	DefaultHealthyTimeout = 90 * time.Second
	// The healthyPollInterval is the poll interval while waiting for readiness.
	healthyPollInterval = time.Second
)

// Instance identifies a long-lived container by its component coordinates
// (stack, component type, and component name) — the same triple FindInstance
// uses for label-based discovery.
type Instance struct {
	Stack         string
	ComponentType string
	Component     string
}

// WaitHealthy blocks until the long-lived container for a component instance
// reports a healthy state, or the timeout elapses. It is meant for containers
// created with a health check: a container with no health check never reports a
// health state, so callers must only invoke this when a health check is
// configured (otherwise it would always time out). An "unhealthy" state — which
// the runtime reports only after the start period and the configured retries —
// fails fast rather than waiting out the timeout.
func WaitHealthy(ctx context.Context, runtime Runtime, inst Instance, timeout time.Duration) error {
	defer perf.Track(nil, "container.WaitHealthy")()

	if runtime == nil {
		return errUtils.ErrNilParam
	}
	if timeout <= 0 {
		timeout = DefaultHealthyTimeout
	}
	address := InstanceAddress(inst.Stack, inst.ComponentType, inst.Component)
	deadline := time.Now().Add(timeout)
	for {
		healthy, err := instanceHealthy(ctx, runtime, inst, address)
		if err != nil {
			return err
		}
		if healthy {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %s did not become healthy within %s", errUtils.ErrContainerNotHealthy, address, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthyPollInterval):
		}
	}
}

// instanceHealthy reports whether the component instance is healthy yet. It
// returns an error only for the terminal "unhealthy" state; a missing instance
// or any non-healthy state is reported as (false, nil) so the caller keeps
// polling.
func instanceHealthy(ctx context.Context, runtime Runtime, inst Instance, address string) (bool, error) {
	info, found, err := FindInstance(ctx, runtime, inst.Stack, inst.ComponentType, inst.Component)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	switch info.Health {
	case "healthy":
		return true, nil
	case "unhealthy":
		return false, fmt.Errorf("%w: %s reported unhealthy", errUtils.ErrContainerNotHealthy, address)
	default:
		return false, nil
	}
}
