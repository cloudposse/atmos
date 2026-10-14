package script

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// serviceError keeps the readable message separate from its classification.
type serviceError struct {
	kind, cause     error
	message, detail string
}

func (e *serviceError) Error() string {
	defer perf.Track(nil, "script.serviceError.Error")()
	return e.message
}

func (e *serviceError) Unwrap() error {
	defer perf.Track(nil, "script.serviceError.Unwrap")()
	return e.cause
}

func (e *serviceError) Is(target error) bool {
	defer perf.Track(nil, "script.serviceError.Is")()

	return target == errUtils.ErrScript || errors.Is(e.kind, target)
}

func (e *serviceError) ErrorDetail() string {
	defer perf.Track(nil, "script.serviceError.ErrorDetail")()
	return e.detail
}

func serviceFailure(kind, cause error, format string, args ...any) error {
	return &serviceError{kind: kind, cause: cause, message: fmt.Sprintf(format, args...)}
}

func withProcessDetail(err error, detail string) error {
	var failure *serviceError
	if errors.As(err, &failure) {
		failure.detail = detail
	}
	return err
}
