package utils

import (
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// schemeLessUserinfoPattern matches the start of an address that has no scheme but opens with
// userinfo, "user:secret@host/path". The user and the password hold no "/" or white space, and the
// password is closed by the first "@". An SCP-style git address ("git@github.com:org/repo.git")
// has no ":" before its "@" and does not match.
var schemeLessUserinfoPattern = regexp.MustCompile(`^[^\s/@]+:[^\s/@]*@`)

// windowsDrivePattern matches the drive of a Windows path, "C:\" or "C:/", which is not a user name.
var windowsDrivePattern = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

// RedactIncludeArgument removes credentials from an include argument or include target before it is
// echoed in an error or a log line. Only URL-shaped values are touched: a local path or an
// SCP-style git address keeps its text, and a URL that cannot be proven free of userinfo is
// replaced entirely (see artifact.RedactURL). A value without a scheme that opens with userinfo
// ("user:secret@host/path", "//user:secret@host/path") is URL-shaped too, and is redacted the same way.
func RedactIncludeArgument(value string) string {
	defer perf.Track(nil, "utils.RedactIncludeArgument")()

	if !isURLShaped(value) {
		return value
	}
	return artifact.RedactURL(value)
}

// isURLShaped reports whether value may be an address that carries userinfo.
func isURLShaped(value string) bool {
	if strings.Contains(value, "://") || strings.HasPrefix(value, "//") {
		return true
	}
	return !windowsDrivePattern.MatchString(value) && schemeLessUserinfoPattern.MatchString(value)
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
