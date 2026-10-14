package step

import (
	"context"
	"errors"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StepDeadline enforces a step's `timeout:` on the work it runs. The zero value, and a nil
// pointer, mean the step has no timeout.
type StepDeadline struct {
	ctx     context.Context
	cancel  context.CancelFunc
	parent  context.Context
	timeout time.Duration
	step    string
}

// StartStepDeadline derives a context limited by the step's `timeout:`. The returned deadline's
// Context() is ctx itself when the step sets no timeout. Callers must call Stop when the step
// ends. A timeout that is not a positive Go duration fails with ErrStepTimeoutInvalid. The vars
// argument renders a templated timeout, and may be nil for a literal duration.
func StartStepDeadline(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepDeadline, error) {
	defer perf.Track(nil, "step.StartStepDeadline")()

	deadline := &StepDeadline{ctx: ctx, parent: ctx, step: step.Name}
	raw := strings.TrimSpace(step.Timeout)
	if raw == "" {
		return deadline, nil
	}
	if vars != nil && !step.IsLiteral("timeout") {
		rendered, err := vars.Resolve(raw)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrStepTimeoutInvalid).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("timeout", raw).
				Err()
		}
		raw = strings.TrimSpace(rendered)
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		builder := errUtils.Build(errUtils.ErrStepTimeoutInvalid).
			WithExplanationf("Step '%s' sets timeout '%s', which is not a positive duration.", step.Name, raw).
			WithHint("Use a Go duration such as `30s`, `5m`, or `1h30m`.").
			WithContext("step", step.Name).
			WithContext("timeout", raw)
		if err != nil {
			builder = builder.WithCause(err)
		}
		return nil, builder.Err()
	}
	deadline.timeout = timeout
	deadline.ctx, deadline.cancel = context.WithTimeout(ctx, timeout)
	return deadline, nil
}

// Context returns the context the step's work must run under.
func (d *StepDeadline) Context() context.Context {
	defer perf.Track(nil, "step.StepDeadline.Context")()

	return d.ctx
}

// Stop releases the timer. It is safe to call more than once.
func (d *StepDeadline) Stop() {
	defer perf.Track(nil, "step.StepDeadline.Stop")()

	if d != nil && d.cancel != nil {
		d.cancel()
	}
}

// Wrap marks err as a step timeout when the step's own deadline, rather than the caller's
// context, ended the work. Any other error, including nil, is returned unchanged.
func (d *StepDeadline) Wrap(err error) error {
	defer perf.Track(nil, "step.StepDeadline.Wrap")()

	if err == nil || d == nil || d.timeout <= 0 {
		return err
	}
	if !errors.Is(d.ctx.Err(), context.DeadlineExceeded) || d.parent.Err() != nil {
		return err
	}
	if errors.Is(err, errUtils.ErrStepTimeout) {
		return err
	}
	return errUtils.Build(errUtils.ErrStepTimeout).
		WithCause(err).
		WithExplanationf("Step '%s' did not finish within its timeout of %s and was canceled.", d.step, d.timeout).
		WithHint("Raise the step's `timeout:` or make the work faster.").
		WithContext("step", d.step).
		WithContext("timeout", d.timeout.String()).
		Err()
}

// RunWithStepDeadline runs fn under the step's `timeout:` and marks a deadline overrun as
// ErrStepTimeout. A step with no timeout runs fn with ctx unchanged. It serves the command paths
// that run a process directly instead of through a step handler.
func RunWithStepDeadline(ctx context.Context, step *schema.WorkflowStep, vars *Variables, fn func(ctx context.Context) error) error {
	defer perf.Track(nil, "step.RunWithStepDeadline")()

	deadline, err := StartStepDeadline(ctx, step, vars)
	if err != nil {
		return err
	}
	defer deadline.Stop()
	return deadline.Wrap(fn(deadline.Context()))
}

// RunWithStepRetry runs fn under one `timeout:` deadline that bounds the whole logical step:
// every retry attempt and every backoff wait between attempts. The deadline context is passed to
// both retry.Do and fn, so an expired deadline stops the loop instead of starting a later attempt,
// and the overrun surfaces as ErrStepTimeout rather than as a retry-exhaustion error. A nil
// step.Retry runs fn once. The vars argument renders a templated timeout and may be nil.
func RunWithStepRetry(ctx context.Context, step *schema.WorkflowStep, vars *Variables, fn func(ctx context.Context) error) error {
	defer perf.Track(nil, "step.RunWithStepRetry")()

	return RunWithStepDeadline(ctx, step, vars, func(stepCtx context.Context) error {
		return retry.Do(stepCtx, step.Retry, func() error {
			return fn(stepCtx)
		})
	})
}

// StepTimeoutBoundsRetries reports whether the step type enforces `timeout:` as a deadline over its
// whole execution, so a caller that retries the step must run the retry loop under that deadline
// (see RunWithStepRetry). Other types give `timeout:` their own meaning: `sleep` uses it as the
// pause length and `http` as a per-request limit.
func StepTimeoutBoundsRetries(stepType string) bool {
	defer perf.Track(nil, "step.StepTimeoutBoundsRetries")()

	switch strings.TrimSpace(stepType) {
	case schema.TaskTypeShell, schema.TaskTypeScript, schema.TaskTypeAtmos, tflintStepType:
		return true
	default:
		return false
	}
}
