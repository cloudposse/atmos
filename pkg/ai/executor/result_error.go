package executor

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/formatter"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ResultError converts an execution result into the error a command should return.
// It returns nil for a successful result that has something to show.
//
// Errors that already explain themselves (user aborts, and provider errors that carry
// hints) are returned unchanged. Other failures are wrapped in ErrAIExecutionFailed.
// A successful result with no text and no tool activity is an error, so the command
// never prints a blank answer.
func ResultError(result *formatter.ExecutionResult) error {
	defer perf.Track(nil, "executor.ResultError")()

	if result == nil {
		return errUtils.ErrAIExecutionFailed
	}

	if !result.Success {
		return failedResultError(result.Error)
	}

	if strings.TrimSpace(result.Response) == "" && len(result.ToolCalls) == 0 {
		return errUtils.Build(errUtils.ErrAIEmptyResponse).
			WithHint("Try again, or ask the question differently").
			WithHint("Run with `--logs-level=Debug` to see what the provider returned").
			Err()
	}

	return nil
}

// selfExplainingErrors are returned to the user as-is because they carry their own context and hints.
var selfExplainingErrors = []error{
	errUtils.ErrUserAborted,
	errUtils.ErrCLIProviderToolDenied,
	errUtils.ErrCLIProviderEmptyResponse,
	errUtils.ErrCLIProviderMaxTurns,
}

func failedResultError(info *formatter.ErrorInfo) error {
	if info == nil {
		return errUtils.ErrAIExecutionFailed
	}

	for _, target := range selfExplainingErrors {
		if errors.Is(info.Err, target) {
			return info.Err
		}
	}

	return fmt.Errorf("%w: %s", errUtils.ErrAIExecutionFailed, info.Message)
}
