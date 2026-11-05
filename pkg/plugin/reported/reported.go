// Package reported marks plugin failures that were already shown to the user.
package reported

import (
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// failure keeps the original error inspectable and adds a silent exit status, so the root
// error handler exits non-zero without rendering the same failure a second time.
type failure struct{ err error }

//nolint:lintroller // Trivial interface accessor with no independent I/O.
func (f *failure) Error() string { return f.err.Error() }

//nolint:lintroller // Trivial interface accessor with no independent I/O.
func (f *failure) Unwrap() []error {
	return []error{f.err, errUtils.ExitCodeError{Code: 1, Silent: true}}
}

// Wrap returns err unchanged for matching with errors.Is and errors.As, but silent at the root
// error handler. Use it only after the failure was rendered through the UI channel or a report.
// A nil error stays nil, and an error that is already reported is not wrapped twice.
func Wrap(err error) error {
	defer perf.Track(nil, "reported.Wrap")()

	if err == nil || IsReported(err) {
		return err
	}
	return &failure{err: err}
}

// IsReported reports whether err was marked by Wrap.
func IsReported(err error) bool {
	defer perf.Track(nil, "reported.IsReported")()

	var marked *failure
	return errors.As(err, &marked)
}
