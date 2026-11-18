package provider

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// redactedQuery replaces a URL query string or fragment inside an error message.
const redactedQuery = "[redacted]"

// queryInMessage matches a query string or fragment inside free-form error text, up to the end of the
// URL: whitespace, a closing quote followed by a colon (the `Get "<url>": ...` form of *url.Error), or
// the end of the message. Escaped quotes inside a %q-quoted URL do not end the match.
var queryInMessage = regexp.MustCompile(`[?#]\S*?("?:\s|\s|"?:?$)`)

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

// redactMessage removes URL query strings and fragments from free-form error text.
func redactMessage(msg string) string {
	return queryInMessage.ReplaceAllString(msg, "?"+redactedQuery+"$1")
}

// RedactError returns err with the URL of an embedded *url.Error redacted like RedactURL.
// Atmos fork: http.Client.Do errors embed the full request URL, including query-string session tokens,
// and they are logged and shown to users. The cause's message is sanitized too, because Go reports a
// malformed redirect Location (which can carry tokens) inside the cause. The redacted error keeps the
// original cause, so errors.Is and errors.As still match it and timeouts are still reported.
func RedactError(err error) error {
	var urlErr *url.Error
	if err == nil || !errors.As(err, &urlErr) {
		return err
	}
	redacted := RedactRawURL(urlErr.URL)
	// Client.Do returns the *url.Error itself, so it is rebuilt with the redacted URL and cause.
	if any(err) == any(urlErr) {
		return &url.Error{Op: urlErr.Op, URL: redacted, Err: redactCause(urlErr.Err)}
	}
	// A wrapper keeps its structure; the URL text in its message is replaced in both its raw and
	// %q-quoted forms, then any remaining query string is removed.
	msg := err.Error()
	msg = strings.ReplaceAll(msg, strconv.Quote(urlErr.URL), strconv.Quote(redacted))
	msg = strings.ReplaceAll(msg, urlErr.URL, redacted)
	return &redactedError{msg: redactMessage(msg), err: err}
}

// redactCause wraps cause so its message carries no URL query strings.
func redactCause(cause error) error {
	if cause == nil {
		return nil
	}
	return &redactedError{msg: redactMessage(cause.Error()), err: cause}
}

// redactedError carries a message with URLs redacted while still unwrapping to the original cause.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }

func (e *redactedError) Unwrap() error { return e.err }

// Timeout reports whether the original cause is a timeout, so (*url.Error).Timeout keeps working.
func (e *redactedError) Timeout() bool {
	var timeout interface{ Timeout() bool }
	return errors.As(e.err, &timeout) && timeout.Timeout()
}
