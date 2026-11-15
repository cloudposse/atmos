package utils

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RedactIncludeArgument removes credentials from an include argument or include target before it is
// echoed in an error or a log line. Only URL-shaped values are touched: a local path or an
// SCP-style git address keeps its text, and a URL that cannot be proven free of userinfo is
// replaced entirely (see artifact.RedactURL).
func RedactIncludeArgument(value string) string {
	defer perf.Track(nil, "utils.RedactIncludeArgument")()

	if !strings.Contains(value, "://") {
		return value
	}
	return artifact.RedactURL(value)
}

// redactedIncludeError is an error whose text has had a credential removed. It keeps the original
// error in its chain so errors.Is and errors.As still see the cause.
type redactedIncludeError struct {
	message string
	err     error
}

func (e *redactedIncludeError) Error() string { return e.message }

func (e *redactedIncludeError) Unwrap() error { return e.err }

// redactIncludeError removes the credential of an include target from the text of an error raised
// while fetching it. Fetch errors from the download and OCI layers quote the reference they were
// given (for example "could not parse reference: user:secret@host/image"), so the include
// argument cannot be redacted once for all of them: the credential is cut out of the final text by
// the same helper that redacts URLs, so both agree on where a userinfo ends.
func redactIncludeError(err error, target string) error {
	if err == nil || !strings.Contains(target, "@") {
		return err
	}
	message := err.Error()
	redacted := artifact.RedactCredentials(message, target)
	if redacted == message {
		return err
	}
	return &redactedIncludeError{message: redacted, err: err}
}
