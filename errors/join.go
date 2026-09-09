package errors

import (
	goerrors "errors"

	"github.com/cockroachdb/errors"
)

// JoinPreservingHints joins errs like the standard library's errors.Join, but
// re-attaches every sub-error's hints directly onto the returned error. This
// exists because cockroachdb/errors' hint traversal (GetAllHints, used by
// the CLI's error formatter) walks a single-cause Unwrap() error chain and
// treats a multi-cause Unwrap() []error error -- exactly what errors.Join
// produces -- as a leaf node, so any WithHint() attached to one of the
// joined errors is otherwise silently unreachable by the formatter once
// joined. Callers that join multiple hint-bearing errors (e.g. one per
// configuration rule) should use this instead of errors.Join directly so
// those hints still reach users.
func JoinPreservingHints(errs ...error) error {
	joined := goerrors.Join(errs...)
	if joined == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, err := range errs {
		for _, hint := range errors.GetAllHints(err) {
			if _, ok := seen[hint]; ok {
				continue
			}
			seen[hint] = struct{}{}
			joined = errors.WithHint(joined, hint)
		}
	}
	return joined
}
