package git

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// StderrSwapper is implemented by providers that support swapping their
// stderr writer for the duration of a single operation, letting callers
// capture subprocess stderr without holding a lock on Provider construction.
type StderrSwapper interface {
	SwapStderr(io.Writer) func()
}

// CaptureStderr runs operation with the provider's stderr swapped to a
// masked capture buffer (when the provider supports StderrSwapper), and
// returns the captured, trimmed text alongside operation's error. The buffer
// is routed through iolib.MaskWriter so captured text is safe to embed in an
// error message: never read RunResult.StderrTail directly for this purpose,
// since that field is documented as bypassing masking.
//
// Providers that do not implement StderrSwapper (e.g. test doubles) run
// operation unmodified and return an empty capture.
func CaptureStderr(provider Provider, operation func() error) (string, error) {
	swapper, ok := provider.(StderrSwapper)
	if !ok {
		return "", operation()
	}

	var stderr bytes.Buffer
	restore := swapper.SwapStderr(iolib.MaskWriter(&stderr))
	defer restore()

	err := operation()
	return strings.TrimSpace(stderr.String()), err
}

// WrapOperationError builds an actionable error from a failed Git operation:
// an action description, the workdir it operated on, the (masked) captured
// stderr text, the underlying error, and an optional hint. Returns nil when
// err is nil.
func WrapOperationError(action, workdir, stderr string, err error, hint string) error {
	if err == nil {
		return nil
	}

	explanation := fmt.Sprintf("Failed to %s.", action)
	if workdir != "" {
		explanation = fmt.Sprintf("Failed to %s in %q.", action, workdir)
	}
	explanation += "\n\nUnderlying error:\n\n```text\n" + err.Error() + "\n```"
	if stderr != "" {
		explanation += "\n\nGit output:\n\n```text\n" + stderr + "\n```"
	}

	builder := errUtils.Build(err).
		WithExplanation(explanation).
		WithExitCode(2)
	if hint != "" {
		builder = builder.WithHint(hint)
	}
	return builder.Err()
}
