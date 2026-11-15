package artifact

import (
	"regexp"
	"strings"
	"unicode"
)

// The redaction helpers in this package are fail-closed. They pass a value through only when it
// matches the small grammar below, which they can prove carries no credential, and replace
// everything else, either the whole value or the whole suspicious token, with [RedactedValue].
//
// An address is "[getter::][scheme://|//]authority[/path][?query][#fragment]". The authority ends
// at the first "/", "?", "#" or "\". Every "@" in the authority ends a userinfo, and the last one
// wins. An "@" after the authority is safe only as the introducer of a complete digest
// ("@sha256:<64 hex digits>") that follows a host and a lower-case repository path, as in
// "ghcr.io/acme/catalog@sha256:...": any other one may close a password that held a "/", "?", "#"
// or "\", so the value cannot be proven free of a credential.
var (
	// The getter prefix pattern matches the forced-getter prefix of a go-getter address, such as
	// "git::" in "git::https://example.com/repo.git". It only matches when a URL scheme follows, so
	// a "::" inside scheme-less userinfo ("user::secret@host") is never mistaken for a prefix.
	getterPrefixPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*::[A-Za-z][A-Za-z0-9+.-]*://`)

	// The scheme pattern matches the "scheme://" that starts an address.
	schemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

	// The authority pattern is the only shape of host that survives redaction: a name, an address,
	// or a bracketed IPv6 literal, with an optional port. Anything else may be a password fragment.
	authorityPattern = regexp.MustCompile(`^(?:[A-Za-z0-9_.-]+|\[[0-9A-Za-z:.%]+\])(?::[0-9]*)?$`)

	// The repository path pattern matches the path in front of the "@" of a digest reference: the
	// lower-case path of an OCI repository with an optional tag. A path that holds anything else
	// might be a password that held a "/".
	repositoryPathPattern = regexp.MustCompile(`^/?[a-z0-9]+(?:[._/-]+[a-z0-9]+)*(?::[A-Za-z0-9_][A-Za-z0-9._-]*)?$`)

	// The SCP pattern matches an SCP-style git address, "user@host:path", whose user carries no
	// password: a ":" before the "@" would make it a credential.
	scpPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[^@\s]*$`)
)

const (
	// The authority terminators end the authority of an address.
	authorityTerminators = `/?#\`

	// The minimum fragment length is the shortest password fragment that is removed on its own from free
	// text. Shorter fragments would mangle ordinary words, and they identify nothing.
	minFragmentLength = 4

	// The scheme separator separates a scheme from its authority.
	schemeSeparator = "://"

	// The file scheme is the only scheme whose authority may be empty.
	fileScheme = "file://"
)

// digestHexLengths holds the length in hex digits of each digest algorithm that may follow an "@".
// Only these well-known algorithms qualify, so a host named like an algorithm is never mistaken for
// one.
var digestHexLengths = map[string]int{
	"sha1":   40,
	"sha224": 56,
	"sha256": 64,
	"sha384": 96,
	"sha512": 128,
	"md5":    32,
	"blake3": 64,
}

// address is an address split at the places that decide whether it carries a credential.
type address struct {
	// prefix is the forced-getter prefix, "git::" for example, or empty.
	prefix string
	// scheme is "scheme://", "//", or empty when the address has none.
	scheme string
	// rest is the address after the scheme.
	rest string
	// authorityEnd is the offset in rest where the authority ends.
	authorityEnd int
	// inside are the offsets in rest of every "@" in the authority.
	inside []int
	// stray are the offsets in rest of every "@" after the authority that does not introduce a
	// digest.
	stray []int
}

// parseAddress splits raw at the getter prefix, the scheme, the authority, and every "@".
func parseAddress(raw string) address {
	a := address{}
	a.prefix, raw = splitGetterPrefix(raw)
	if loc := schemePattern.FindStringIndex(raw); loc != nil {
		a.scheme, a.rest = raw[:loc[1]], raw[loc[1]:]
	} else if strings.HasPrefix(raw, "//") {
		a.scheme, a.rest = "//", raw[2:]
	} else {
		a.rest = raw
	}
	a.authorityEnd = strings.IndexAny(a.rest, authorityTerminators)
	if a.authorityEnd < 0 {
		a.authorityEnd = len(a.rest)
	}
	for i := 0; i < a.authorityEnd; i++ {
		if a.rest[i] == '@' {
			a.inside = append(a.inside, i)
		}
	}
	for i := a.authorityEnd; i < len(a.rest); i++ {
		if a.rest[i] == '@' && !a.introducesDigest(i) {
			a.stray = append(a.stray, i)
		}
	}
	return a
}

// introducesDigest reports whether the "@" at offset i of the remainder, after the authority,
// introduces a digest: a complete digest follows it, the host in front is shaped like a host, and
// the path in front is the lower-case path of a repository.
func (a *address) introducesDigest(i int) bool {
	hostStart := 0
	if len(a.inside) > 0 {
		hostStart = a.inside[len(a.inside)-1] + 1
	}
	return digestFollows(a.rest[i+1:]) && a.hostIsSafe(hostStart) && repositoryPathPattern.MatchString(a.rest[a.authorityEnd:i])
}

// boundaries returns the offset of every "@" that may end a userinfo, in order.
func (a *address) boundaries() []int {
	out := make([]int, 0, len(a.inside)+len(a.stray))
	out = append(out, a.inside...)
	return append(out, a.stray...)
}

// digestFollows reports whether s, the text after an "@", starts with a complete digest such as
// "sha256:<64 hex digits>" that is not continued by further letters or digits.
func digestFollows(s string) bool {
	algorithm, rest, found := strings.Cut(s, ":")
	length, known := digestHexLengths[algorithm]
	if !found || !known || len(rest) < length {
		return false
	}
	for i := 0; i < length; i++ {
		if !isHexDigit(rest[i]) {
			return false
		}
	}
	return len(rest) == length || !isAlphanumeric(rest[length])
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func isAlphanumeric(c byte) bool {
	return isHexDigit(c) || c >= 'g' && c <= 'z' || c >= 'G' && c <= 'Z'
}

// hostIsSafe reports whether the host part of the address, the authority after any userinfo, has
// the shape of a host name or address. A host that has not is not echoed: it may be a password
// fragment that a delimiter cut loose.
func (a *address) hostIsSafe(start int) bool {
	host := a.rest[start:a.authorityEnd]
	if host == "" {
		return a.scheme == fileScheme
	}
	return authorityPattern.MatchString(host)
}

// headOf returns s up to its first white space, the part of an include argument that is a URL.
func headOf(s string) string {
	if i := strings.IndexFunc(s, unicode.IsSpace); i >= 0 {
		return s[:i]
	}
	return s
}

// splitGetterPrefix separates a forced-getter prefix ("git::") from the URL that follows it.
func splitGetterPrefix(raw string) (prefix, body string) {
	if !getterPrefixPattern.MatchString(raw) {
		return "", raw
	}
	cut := strings.Index(raw, "::") + len("::")
	return raw[:cut], raw[cut:]
}
