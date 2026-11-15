package artifact

import (
	"net/url"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RedactedValue replaces a value that must not be echoed because it may carry credentials.
const RedactedValue = "<redacted>"

// RedactURL returns raw with any userinfo removed, so that a mistakenly pasted credential is not
// repeated in an error message or a log line. It is the one implementation every caller shares.
//
// RedactURL is fail-closed. A value without an "@" cannot hold userinfo and is returned as it is.
// A value whose "@" signs all sit in the authority has its userinfo cut at the last one, as
// RFC 3986 requires, and is returned when what is left is a well-formed host. A value that cannot
// be proven free of a credential is replaced by [RedactedValue] as a whole: an "@" after the
// authority that does not introduce a complete digest (a password that held "/", "?", "#" or "\"
// may have closed there), scheme-less userinfo such as "user:secret@registry.example", a host that
// is not shaped like a host, an address that does not parse, or an address with more "@" signs
// than a real one holds or longer than any real one (see [maxBoundaries] and [maxAddressLength]),
// which is withheld before any work that grows with it starts. The diagnostic value of an error
// drops for such inputs by design; a credential is never echoed. A forced-getter prefix such as
// "git::" is kept.
func RedactURL(raw string) string {
	defer perf.Track(nil, "artifact.RedactURL")()

	if !strings.Contains(raw, "@") {
		return raw
	}
	a := parseAddress(raw)
	if a.overflow || len(a.stray) > 0 {
		return RedactedValue
	}
	if len(a.inside) == 0 {
		// Only digest introducers: the address carries no userinfo, but it is still checked.
		if a.wellFormed(0) {
			return raw
		}
		return RedactedValue
	}
	// Scheme-less userinfo cannot be told from a host and a path, so it is never cut out.
	if a.scheme == "" {
		return RedactedValue
	}
	start := a.inside[len(a.inside)-1] + 1
	if !a.wellFormed(start) {
		return RedactedValue
	}
	return a.prefix + a.scheme + a.rest[start:]
}

// wellFormed reports whether the address from offset start of its remainder on has a safe host and
// parses as a URL.
func (a *address) wellFormed(start int) bool {
	if !a.hostIsSafe(start) {
		return false
	}
	if a.scheme == "" {
		return true
	}
	_, err := url.Parse(headOf(a.scheme + a.rest[start:]))
	return err == nil
}

// RedactCredentials removes credentials from text, an error message or log line that may quote
// raw, its userinfo, its password on its own, or any other URL. It is fail-closed, in three steps:
//
//  1. The credential carried by raw is cut out of text wherever it is quoted: the userinfo with its
//     trailing "@", written as in a URL, decoded, or escaped for a quote, and the password on its
//     own together with every fragment of it that a delimiter cut loose. The password is the part
//     of the userinfo as written after its first ":", decoded separately. A user name without a
//     password is not replaced when it stands alone, since it is usually a service account such as
//     "git".
//  2. Every other URL-shaped word of text that still holds an "@" which cannot be proven to
//     introduce a digest is replaced by [RedactedValue] as a whole, and so is every word that still
//     holds an "@" when raw carried a credential.
//  3. If any part of the password is still present, the whole text is replaced by [RedactedValue].
//
// A raw address with more "@" signs than a real one holds, or longer than any real one, has no
// credential that can be listed at a bounded cost, so the whole text is replaced by [RedactedValue]
// without taking any of the steps above.
func RedactCredentials(text, raw string) string {
	defer perf.Track(nil, "artifact.RedactCredentials")()

	a := parseAddress(raw)
	if a.overflow {
		// The credential of an address that is past the bounds cannot be listed, so nothing in the
		// text can be proven free of it.
		return RedactedValue
	}
	secret := a.credential()
	if !secret.empty() {
		text = secret.scrub(text)
	}
	text = redactWords(text, !secret.empty())
	if secret.survives(text) {
		return RedactedValue
	}
	return text
}

// RedactError returns err with its message scrubbed of the credentials of every address in raws
// and of any other URL-shaped word that carries userinfo (see [RedactCredentials]). The result keeps
// err in its chain, so errors.Is and errors.As still see the cause, and it is err itself when the
// message needs no change.
func RedactError(err error, raws ...string) error {
	defer perf.Track(nil, "artifact.RedactError")()

	if err == nil {
		return nil
	}
	message := err.Error()
	scrubbed := message
	for _, raw := range raws {
		scrubbed = RedactCredentials(scrubbed, raw)
	}
	if len(raws) == 0 {
		scrubbed = RedactCredentials(scrubbed, "")
	}
	if scrubbed == message {
		return err
	}
	return &redactedError{message: scrubbed, err: err}
}

// RedactDecodeError is [RedactError] for the error of decoding raw, a configuration value as it was
// parsed: a decode error quotes the keys and values it rejects, so every string held by raw, keys
// included and at any depth, is a credential candidate when the message is scrubbed.
func RedactDecodeError(err error, raw any) error {
	defer perf.Track(nil, "artifact.RedactDecodeError")()

	if err == nil {
		return nil
	}
	return RedactError(err, stringValues(raw)...)
}

// redactedError is an error whose text has had a credential removed. It keeps the original error in
// its chain so errors.Is and errors.As still see the cause.
type redactedError struct {
	message string
	err     error
}

//nolint:lintroller // Trivial getter with no I/O.
func (e *redactedError) Error() string { return e.message }

//nolint:lintroller // Trivial getter with no I/O.
func (e *redactedError) Unwrap() error { return e.err }

// IsSCPAddress reports whether s is an SCP-style git address such as "git@github.com:org/repo.git".
// Its user carries no password, so it holds no credential.
func IsSCPAddress(s string) bool {
	defer perf.Track(nil, "artifact.IsSCPAddress")()

	return scpPattern.MatchString(s)
}

// SplitGetterPrefix separates a forced-getter prefix ("git::") from the URL that follows it. The
// prefix is recognized only when a URL scheme follows it, so a "::" inside userinfo is never taken
// for one. The prefix is empty, and body is raw, when there is none.
func SplitGetterPrefix(raw string) (prefix, body string) {
	defer perf.Track(nil, "artifact.SplitGetterPrefix")()

	return splitGetterPrefix(raw)
}
