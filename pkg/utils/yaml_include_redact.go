package utils

import (
	neturl "net/url"
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
// argument cannot be redacted once for all of them: the userinfo is cut out of the final text.
func redactIncludeError(err error, target string) error {
	if err == nil || !strings.Contains(target, "@") {
		return err
	}
	message := err.Error()
	redacted := message
	for _, userinfo := range includeUserinfo(target) {
		redacted = strings.ReplaceAll(redacted, userinfo.full+"@", "")
		for _, secret := range userinfo.secrets() {
			redacted = strings.ReplaceAll(redacted, secret, artifact.RedactedValue)
		}
	}
	if redacted == message {
		return err
	}
	return &redactedIncludeError{message: redacted, err: err}
}

// includeCredential is the userinfo of one URL found in an include target.
type includeCredential struct {
	full   string
	secret string
}

// secrets returns the password as written and as a URL parser decodes it, since a fetch error may
// quote either form.
func (c includeCredential) secrets() []string {
	if c.secret == "" {
		return nil
	}
	secrets := []string{c.secret}
	if decoded, err := neturl.PathUnescape(c.secret); err == nil && decoded != c.secret && decoded != "" {
		secrets = append(secrets, decoded)
	}
	return secrets
}

// includeUserinfo extracts the userinfo of every URL in an include target, including the URL that
// follows a forced-getter prefix such as "git::https://".
func includeUserinfo(target string) []includeCredential {
	var found []includeCredential
	for _, part := range strings.Split(target, "::") {
		_, rest, ok := strings.Cut(part, "://")
		if !ok {
			continue
		}
		authority, _, _ := strings.Cut(rest, "/")
		userinfo, _, hasUser := strings.Cut(authority, "@")
		if !hasUser {
			continue
		}
		_, secret, _ := strings.Cut(userinfo, ":")
		found = append(found, includeCredential{full: userinfo, secret: secret})
	}
	return found
}
