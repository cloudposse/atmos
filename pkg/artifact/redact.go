package artifact

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RedactedValue replaces a value that must not be echoed because it may carry credentials.
const RedactedValue = "<redacted>"

var (
	// The getter prefix pattern matches the forced-getter prefix of a go-getter address, such as
	// "git::" in "git::https://example.com/repo.git". It only matches when a URL scheme follows, so
	// a "::" inside scheme-less userinfo ("user::secret@host") is never mistaken for a prefix.
	getterPrefixPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*::[A-Za-z][A-Za-z0-9+.-]*://`)

	// The digest pattern matches a digest ("sha256:<hex>") that follows the "@" of a digest reference.
	// Only well-known hash algorithms qualify, so a host named like an algorithm is never mistaken
	// for one. An "@" followed by a digest introduces content, not a credential.
	digestPattern = regexp.MustCompile(`^(?:sha1|sha224|sha256|sha384|sha512|md5|blake3):[0-9a-fA-F]+`)
)

// RedactURL returns raw with any userinfo removed, so that a mistakenly pasted credential is not
// repeated in an error message. Strings that cannot be proven free of userinfo are replaced by
// [RedactedValue] instead of being returned verbatim: a value that fails to parse, that carries
// an "@" outside a URL authority (for example "user:secret@registry.example"), or whose "@" is not
// the start of a digest, is never echoed. The last "@" of the authority ends the userinfo, as
// RFC 3986 requires, so a password containing "@" or "::" is removed as a whole. A forced-getter
// prefix such as "git::" is kept.
func RedactURL(raw string) string {
	defer perf.Track(nil, "artifact.RedactURL")()

	if !strings.Contains(raw, "@") {
		return raw
	}
	prefix, body := splitGetterPrefix(raw)
	parsed, err := url.Parse(body)
	if err != nil {
		return RedactedValue
	}
	if parsed.User != nil {
		parsed.User = nil
		return prefix + parsed.String()
	}
	if parsed.Host == "" || len(credentialBoundaries(afterScheme(body))) > 0 {
		// An "@" without an authority is scheme-less userinfo such as "user:secret@host", and an "@"
		// after the authority that is not a digest may close a password holding "/", "?" or "#".
		return RedactedValue
	}
	return raw
}

// RedactCredentials removes the credential carried by the URL raw from text, an error message that
// may quote raw, its userinfo, or its password on its own. The userinfo and its trailing "@" are
// cut out, and the password is replaced by [RedactedValue] in the form written and in the form a
// URL parser decodes. A user name without a password is not replaced when it stands alone, since it
// is usually a service account such as "git". The credential is found structurally, never by
// splitting on ":" or "::", so a password holding ":", "::", "@", "/", "?" or "#" is removed whole.
func RedactCredentials(text, raw string) string {
	defer perf.Track(nil, "artifact.RedactCredentials")()

	if !strings.Contains(raw, "@") {
		return text
	}
	_, body := splitGetterPrefix(raw)
	rest := afterScheme(body)
	boundaries := credentialBoundaries(rest)
	// Longest userinfo first, so a shorter prefix never leaves the tail of a longer password behind.
	sort.Sort(sort.Reverse(sort.IntSlice(boundaries)))
	for _, boundary := range boundaries {
		text = redactUserinfo(text, rest[:boundary])
	}
	return text
}

// redactUserinfo removes one userinfo, written as in a URL, from text.
func redactUserinfo(text, userinfo string) string {
	forms := []string{userinfo}
	if decoded, err := url.PathUnescape(userinfo); err == nil && decoded != userinfo {
		forms = append(forms, decoded)
	}
	for _, form := range forms {
		text = strings.ReplaceAll(text, form+"@", "")
	}
	for _, form := range forms {
		if _, password, ok := strings.Cut(form, ":"); ok && password != "" {
			text = strings.ReplaceAll(text, password, RedactedValue)
		}
	}
	return text
}

// SplitGetterPrefix separates a forced-getter prefix ("git::") from the URL that follows it. The
// prefix is recognized only when a URL scheme follows it, so a "::" inside userinfo is never taken
// for one. The prefix is empty, and body is raw, when there is none.
func SplitGetterPrefix(raw string) (prefix, body string) {
	defer perf.Track(nil, "artifact.SplitGetterPrefix")()

	return splitGetterPrefix(raw)
}

// splitGetterPrefix is [SplitGetterPrefix] without the performance tracking, for internal callers.
func splitGetterPrefix(raw string) (prefix, body string) {
	loc := getterPrefixPattern.FindStringIndex(raw)
	if loc == nil {
		return "", raw
	}
	cut := strings.Index(raw, "::") + len("::")
	return raw[:cut], raw[cut:]
}

// afterScheme returns the text after the scheme separator "://", after a leading "//" of a
// scheme-relative reference, or s itself when it has neither.
func afterScheme(s string) string {
	if _, rest, ok := strings.Cut(s, "://"); ok {
		return rest
	}
	return strings.TrimPrefix(s, "//")
}

// credentialBoundaries returns the offset of every "@" in rest that may end a userinfo: any "@"
// that is not the start of a digest. A userinfo is cut at the last "@" of the authority, but a
// password may itself hold "/", "?" or "#", which would end the authority early, so every "@"
// that cannot be proven to introduce a digest counts as a boundary.
func credentialBoundaries(rest string) []int {
	var boundaries []int
	for i := 0; i < len(rest); i++ {
		if rest[i] == '@' && !digestPattern.MatchString(rest[i+1:]) {
			boundaries = append(boundaries, i)
		}
	}
	return boundaries
}
