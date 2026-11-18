package redacttest

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Input is one generated credential-bearing address and the ways an error may quote it.
type Input struct {
	// Name describes the template the address was built from.
	Name string
	// Raw is the address.
	Raw string
	// Password is the password as it was typed into Raw, or empty when Raw has none.
	Password string
}

// Messages returns error texts that quote the input the way fetch, parse, and decode errors do:
// verbatim, quoted with %q, stripped of its scheme, as the password on its own, and as the text of
// a URL parse error.
func (in Input) Messages() []string {
	defer perf.Track(nil, "redacttest.Input.Messages")()

	_, withoutScheme, _ := strings.Cut(in.Raw, "://")
	messages := []string{
		"fetch " + in.Raw + " failed: denied",
		fmt.Sprintf("fetch %q failed: denied", in.Raw),
		"could not parse reference: " + withoutScheme,
		fmt.Sprintf("Get %q: dial tcp: lookup failed", in.Raw),
	}
	if _, err := url.Parse(in.Raw); err != nil {
		messages = append(messages, err.Error())
	}
	if in.Password != "" {
		messages = append(messages, "authentication failed for "+in.Password, fmt.Sprintf("password %q rejected", in.Password))
	}
	return messages
}

// Leaks returns what survives in out of the user name and of the secret: every fragment of a
// secret starts with [Marker], so a delimiter inside the secret cannot hide a leak.
func Leaks(out string) []string {
	defer perf.Track(nil, "redacttest.Leaks")()

	var leaks []string
	for _, needle := range []string{Marker, User} {
		if strings.Contains(out, needle) {
			leaks = append(leaks, needle)
		}
	}
	return leaks
}

// RequireNoLeak fails the test when out, the redacted output of the entry point named entry for
// in, still holds the user name or any fragment of the secret.
func RequireNoLeak(t testing.TB, entry string, in Input, out string) {
	defer perf.Track(nil, "redacttest.RequireNoLeak")()

	t.Helper()

	if leaks := Leaks(out); len(leaks) > 0 {
		t.Fatalf("%s leaked %q for %s input %q: %q", entry, leaks, in.Name, in.Raw, out)
	}
}

// Seeds returns the separators a fuzz run starts from: one for every delimiter the shared table
// splits a password at, so that the corpus covers every input of [Cases] as a generated secret.
func Seeds() []string {
	defer perf.Track(nil, "redacttest.Seeds")()

	return []string{"", ":", "::", "@", "@@", "#", "?", "/", "%40", "%zz", " ", "\t", "[", "]", "\\", "\u2713", "@" + digestHex, "://", "@sha256:443/"}
}

// digestHex is a complete sha256 digest, so a template can place a real digest next to a credential.
var digestHex = "sha256:" + strings.Repeat("0f", digestBytes)

// Generate embeds secret as the password of many address templates: user information of every
// scheme, with the secret before and after other delimiters, in an escaped form, repeated in the
// query and the fragment, and with hosts named like digests. Each address ends its user
// information with an "@" of its own, so the secret is always a credential and never a host.
// The secret may hold any delimiter, so the same template covers a password that closes the
// authority early.
func Generate(secret string) []Input {
	defer perf.Track(nil, "redacttest.Generate")()

	escaped := url.PathEscape(secret)
	const (
		https = "https://"
		colon = ":"
		at    = "@"
		host  = "@example.com/a"
	)
	credential := User + colon + secret
	templates := []struct{ name, raw, password string }{
		{"https", https + credential + host, secret},
		{"upper-case scheme with a port, a query and a fragment", "HTTPS://" + credential + "@example.com:8443/a?x=1#f", secret},
		{"scheme-relative", "//" + credential + host, secret},
		{"scheme-less", credential + host, secret},
		{"forced getter", "git::" + https + credential + "@example.com/r.git//sub?ref=v1", secret},
		{"oci with a digest", "oci://" + credential + "@ghcr.io/acme/catalog@" + digestHex, secret},
		{"host named like an algorithm with a port", https + credential + "@sha256:443/a", secret},
		{"host named like a digest", https + credential + at + digestHex + "/a", secret},
		{"digest in the user information", https + credential + at + digestHex + host, secret},
		{"ipv6 host", https + credential + "@[::1]:8443/a", secret},
		{"token as the user", https + secret + host, ""},
		{"credential then a digest", https + credential + "@example.com/a@" + digestHex, secret},
		{"credential repeated in the query", https + credential + "@example.com/a?next=" + https + credential + "@other.example/b", secret},
		{"credential repeated in the fragment", https + credential + "@example.com/a#" + https + credential + "@other.example/b", secret},
		{"escaped password", https + User + colon + escaped + host, secret},
		{"escaped colon in the user", https + User + "%3A" + secret + colon + escaped + host, secret},
		{"two userinfos", https + credential + at + secret + host, secret},
		{"ssh", "ssh://" + credential + "@example.com:22/r.git", secret},
		{"no host", https + credential + "@", secret},
	}
	inputs := make([]Input, 0, len(templates))
	for _, tmpl := range templates {
		inputs = append(inputs, Input{Name: tmpl.name, Raw: tmpl.raw, Password: tmpl.password})
	}
	return inputs
}

// SecretOf builds a secret that carries [Marker] twice, joined by an arbitrary separator, so a
// delimiter inside the secret splits it into fragments that each start with the marker.
func SecretOf(first, separator, second string) string {
	defer perf.Track(nil, "redacttest.SecretOf")()

	return Marker + first + separator + Marker + second
}
