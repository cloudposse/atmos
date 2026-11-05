package hooks

import (
	"strings"

	"github.com/cockroachdb/errors"
)

// logKeyError and logKeyHint are the structured-log keys for a hook failure's
// message and its remediation hints.
const (
	logKeyError = "error"
	logKeyHint  = "hint"
)

// failureLogFields returns structured-log key/value pairs describing err by its
// message and any user-facing hints. Passing the error value itself to the
// logger would render the full cockroachdb error chain (wrapper types, marks,
// and "Error types:" lines), which is diagnostic noise in a one-line warning.
func failureLogFields(err error) []any {
	fields := []any{logKeyError, err.Error()}
	var hints []string
	for _, hint := range errors.GetAllHints(err) {
		if strings.HasPrefix(hint, "TITLE:") || strings.HasPrefix(hint, "EXAMPLE:") {
			continue
		}
		hints = append(hints, hint)
	}
	if len(hints) > 0 {
		fields = append(fields, logKeyHint, strings.Join(hints, "; "))
	}
	return fields
}
