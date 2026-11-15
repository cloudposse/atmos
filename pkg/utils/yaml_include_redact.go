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

// redactIncludeError removes the credential of an include target from the text of an error raised
// while fetching it. Fetch errors from the download and OCI layers quote the reference they were
// given (for example "could not parse reference: user:secret@host/image"), so the include
// argument cannot be redacted once for all of them: the credential is cut out of the final text by
// the helper that redacts URLs, so both agree on where a userinfo ends, and any other URL that
// carries userinfo is withheld as well (see artifact.RedactError).
func redactIncludeError(err error, target string) error {
	return artifact.RedactError(err, target)
}
