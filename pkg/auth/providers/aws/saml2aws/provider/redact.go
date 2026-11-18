package provider

import "net/url"

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
