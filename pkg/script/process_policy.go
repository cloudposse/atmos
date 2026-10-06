package script

import (
	"context"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/retry"
)

func runProcessWithPolicy(ctx context.Context, runner process.Runner, call *ProcessCall) (ProcessOutput, error) {
	var output ProcessOutput
	var retryable bool
	var conditions []string
	if call.Policy.Retry != nil {
		conditions = call.Policy.Retry.Conditions
	}
	patterns, err := retry.CompileConditions(conditions)
	if err != nil {
		return output, serviceFailure(errUtils.ErrScriptInvalidArgument, err, "invalid retry conditions: %s", err)
	}
	err = call.Policy.Execute(ctx, func(attemptContext context.Context) error {
		var attemptError error
		output, retryable, attemptError = runProcessOnce(attemptContext, runner, call)
		return attemptError
	}, func(err error) bool {
		if !retryable || !errors.Is(err, errUtils.ErrScriptProcessFailed) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		return len(patterns) == 0 || retry.MatchesAny(patterns, output.Stdout+"\n"+output.Stderr)
	})
	return output, err
}
