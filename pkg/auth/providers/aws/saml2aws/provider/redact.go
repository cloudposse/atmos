package provider

import (
	"errors"
	"net/url"
	"strings"
)

// RedactURL returns u for logging: scheme, host and path only.
// Atmos fork: identity-provider URLs can carry session tokens and one-time codes in the
// query string or fragment (for example Okta's session-cookie redirect), and user info in
// the authority, so none of those are logged.
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}

// RedactRawURL parses raw and returns it redacted like RedactURL.
// A value that does not parse as a URL is not logged.
func RedactRawURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[unparseable URL]"
	}
	return RedactURL(u)
}

// RedactError returns err with the URL of an embedded *url.Error redacted like RedactURL.
// Atmos fork: http.Client.Do errors embed the full request URL, including query-string session tokens,
// and they are logged and shown to users. The redacted error keeps the wrapped cause, so errors.Is and
// errors.As still match it.
func RedactError(err error) error {
	var urlErr *url.Error
	if err == nil || !errors.As(err, &urlErr) {
		return err
	}
	redacted := RedactRawURL(urlErr.URL)
	// Client.Do returns the *url.Error itself, so it is rebuilt with the redacted URL.
	if any(err) == any(urlErr) {
		return &url.Error{Op: urlErr.Op, URL: redacted, Err: urlErr.Err}
	}
	// A wrapper keeps its structure; only the URL text in its message is replaced.
	return &redactedError{msg: strings.ReplaceAll(err.Error(), urlErr.URL, redacted), err: err}
}

// redactedError carries a message with URLs redacted while still unwrapping to the original cause.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }

func (e *redactedError) Unwrap() error { return e.err }
