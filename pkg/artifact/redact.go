package artifact

import (
	"net/url"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RedactedValue replaces a value that must not be echoed because it may carry credentials.
const RedactedValue = "<redacted>"

// RedactURL returns raw with any userinfo removed, so that a mistakenly pasted credential is not
// repeated in an error message. Strings that cannot be proven free of userinfo are replaced by
// [RedactedValue] instead of being returned verbatim: a value that fails to parse, or that carries
// an "@" outside a URL authority (for example "user:secret@registry.example"), is never echoed.
func RedactURL(raw string) string {
	defer perf.Track(nil, "artifact.RedactURL")()

	if !strings.Contains(raw, "@") {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return RedactedValue
	}
	if parsed.User != nil {
		parsed.User = nil
		return parsed.String()
	}
	if parsed.Host == "" {
		// An "@" without an authority is scheme-less userinfo such as "user:secret@host".
		return RedactedValue
	}
	return raw
}
