package container

import (
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// validRestartPolicies is the set of restart policies accepted by docker/podman.
var validRestartPolicies = map[string]struct{}{
	"no":             {},
	"always":         {},
	"on-failure":     {},
	"unless-stopped": {},
}

// RestartPolicyFromStep maps a run step's restart spec onto a runtime
// RestartPolicy, or nil when none is configured (the runtime then uses its
// default of `no`). It is shared by the container component and the emulator
// kind so both translate `restart:` identically.
func RestartPolicyFromStep(step *schema.ContainerRunStep) *RestartPolicy {
	defer perf.Track(nil, "container.RestartPolicyFromStep")()

	if step == nil || step.Restart == nil || step.Restart.Policy == "" {
		return nil
	}
	return &RestartPolicy{
		Policy:     step.Restart.Policy,
		MaxRetries: step.Restart.MaxRetries,
	}
}

// HealthCheckFromStep maps a run step's healthcheck spec onto a runtime
// HealthCheck, resolving the Compose `test` form (a bare string or a list whose
// first element is `NONE`, `CMD`, or `CMD-SHELL`) into a single shell command.
// Returns nil when no healthcheck is configured (the container then inherits any
// image healthcheck). It is shared by the container component and the emulator
// kind so both translate `healthcheck:` identically.
func HealthCheckFromStep(step *schema.ContainerRunStep) *HealthCheck {
	defer perf.Track(nil, "container.HealthCheckFromStep")()

	if step == nil || step.HealthCheck == nil {
		return nil
	}
	hc := step.HealthCheck
	cmd, disable := resolveHealthTest(hc.Test)
	if hc.Disable {
		disable = true
	}
	if disable {
		return &HealthCheck{Disable: true}
	}
	return &HealthCheck{
		Cmd:           cmd,
		Interval:      hc.Interval,
		Timeout:       hc.Timeout,
		Retries:       hc.Retries,
		StartPeriod:   hc.StartPeriod,
		StartInterval: hc.StartInterval,
	}
}

// resolveHealthTest resolves a Compose `test` value into a single shell command
// for `--health-cmd`. The leading `NONE`/`CMD`/`CMD-SHELL` token is interpreted
// per Compose: `NONE` disables the check; `CMD` and `CMD-SHELL` strip the prefix;
// an unprefixed value (or bare string) is treated as `CMD-SHELL`. The CLI runs
// `--health-cmd` via the shell, so `CMD` exec-form args are joined with spaces.
func resolveHealthTest(test []string) (cmd string, disable bool) {
	if len(test) == 0 {
		return "", false
	}
	switch {
	case strings.EqualFold(test[0], "NONE"):
		return "", true
	case strings.EqualFold(test[0], "CMD"), strings.EqualFold(test[0], "CMD-SHELL"):
		return strings.Join(test[1:], " "), false
	default:
		return strings.Join(test, " "), false
	}
}

// ValidateRunStep checks a run step's restart/healthcheck settings up front so a
// misconfiguration surfaces as a friendly Atmos error instead of an opaque
// docker/podman failure at create time. It is a no-op when step is nil.
func ValidateRunStep(step *schema.ContainerRunStep) error {
	defer perf.Track(nil, "container.ValidateRunStep")()

	if step == nil {
		return nil
	}
	if r := step.Restart; r != nil && r.Policy != "" {
		if _, ok := validRestartPolicies[r.Policy]; !ok {
			return fmt.Errorf("%w: %q (want one of: no, always, on-failure, unless-stopped)",
				errUtils.ErrInvalidContainerRestartPolicy, r.Policy)
		}
		if r.MaxRetries < 0 {
			return fmt.Errorf("%w: max_retries must not be negative", errUtils.ErrInvalidContainerRestartPolicy)
		}
	}
	return validateHealthCheckStep(step.HealthCheck)
}

// validateHealthCheckStep validates the healthcheck durations and retry count.
func validateHealthCheckStep(hc *schema.ContainerHealthCheck) error {
	if hc == nil {
		return nil
	}
	if hc.Retries < 0 {
		return fmt.Errorf("%w: retries must not be negative", errUtils.ErrInvalidContainerHealthCheck)
	}
	durations := map[string]string{
		"interval":       hc.Interval,
		"timeout":        hc.Timeout,
		"start_period":   hc.StartPeriod,
		"start_interval": hc.StartInterval,
	}
	for field, value := range durations {
		if value == "" {
			continue
		}
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%w: %s %q is not a valid duration (e.g. 30s, 1m30s): %w",
				errUtils.ErrInvalidContainerHealthCheck, field, value, err)
		}
	}
	return nil
}
