package artifact

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/artifact/redacttest"
)

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "no userinfo", raw: "oci://ghcr.io/acme/catalog:stable", want: "oci://ghcr.io/acme/catalog:stable"},
		{name: "digest reference keeps its at sign", raw: "oci://ghcr.io/acme/catalog@" + sha256Digest, want: "oci://ghcr.io/acme/catalog@" + sha256Digest},
		{name: "a digest that is not complete is not a digest", raw: "oci://ghcr.io/acme/catalog@sha256:abc", want: RedactedValue},
		{name: "unknown algorithm is not a digest", raw: "oci://ghcr.io/acme/catalog@sha999:" + strings.Repeat("0f", 32), want: RedactedValue},
		{name: "host named like a digest algorithm", raw: "https://user:hunter2@sha256:443/a", want: "https://sha256:443/a"},
		{name: "scheme-less digest reference", raw: "ghcr.io/acme/catalog@" + sha256Digest, want: "ghcr.io/acme/catalog@" + sha256Digest},
		{name: "scheme-less digest of a bare name", raw: "catalog@" + sha256Digest, want: RedactedValue},
		{name: "at sign after the authority is not a digest", raw: "https://user:hunter2/x@example.com/a", want: RedactedValue},
		{name: "at sign in the query", raw: "https://example.com/a?mail=user@example.com", want: RedactedValue},
		{name: "host that is not shaped like a host", raw: "https://user:hunter2@exa mple.com/a", want: RedactedValue},
		{name: "user and password", raw: "oci://user:hunter2@ghcr.io/acme/catalog:stable", want: "oci://ghcr.io/acme/catalog:stable"},
		{name: "token as user", raw: "https://hunter2@issuer.example", want: "https://issuer.example"},
		{name: "userinfo and digest", raw: "oci://user:hunter2@ghcr.io/acme/catalog@" + sha256Digest, want: "oci://ghcr.io/acme/catalog@" + sha256Digest},
		{name: "forced getter prefix is kept", raw: "git::https://user:hunter2@example.com/r.git", want: "git::https://example.com/r.git"},
		{name: "scheme-relative userinfo", raw: "//user:hunter2@example.com/a", want: "//example.com/a"},
		{name: "password with an at sign", raw: "https://user:pa@ss@example.com/a", want: "https://example.com/a"},
		{name: "malformed escape with userinfo", raw: "oci://user:hunter2@ghcr.io/acme/%zz:stable", want: RedactedValue},
		{name: "scheme-less userinfo", raw: "user:hunter2@ghcr.io", want: RedactedValue},
		{name: "scheme-less userinfo and port", raw: "user:hunter2@ghcr.io:5000", want: RedactedValue},
		{name: "malformed without an at sign is not credential bearing", raw: "oci://ghcr.io/%zz", want: "oci://ghcr.io/%zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactURL(tt.raw)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "hunter2")
		})
	}
}

// TestRedact_SharedCredentialCases feeds the shared table to every redaction entry point of this
// package and checks the one security property they share: no user or password of a
// credential-bearing input survives, whichever way an error quotes it.
func TestRedact_SharedCredentialCases(t *testing.T) {
	for _, tc := range redacttest.Cases() {
		t.Run(tc.Name, func(t *testing.T) {
			got := RedactURL(tc.Raw)
			for _, secret := range tc.Secrets {
				assert.NotContains(t, got, secret, "RedactURL of %q", tc.Raw)
			}
			messages := []string{"fetch " + tc.Raw + " failed: denied", fmt.Sprintf("fetch %q failed", tc.Raw), "bad: " + got}
			for _, quoted := range tc.Quoted {
				messages = append(messages, "authentication failed for "+quoted)
				messages = append(messages, "could not parse reference: "+tc.Raw+" (password "+quoted+")")
				messages = append(messages, fmt.Sprintf("password %q rejected", quoted))
			}
			for _, message := range messages {
				scrubbed := RedactCredentials(message, tc.Raw)
				for _, secret := range tc.Secrets {
					assert.NotContains(t, scrubbed, secret, "RedactCredentials of %q", message)
				}
				err := RedactError(fmt.Errorf("%w: %s", errSentinel, message), tc.Raw)
				for _, secret := range tc.Secrets {
					assert.NotContains(t, err.Error(), secret, "RedactError of %q", message)
				}
				assert.ErrorIs(t, err, errSentinel, "the cause stays in the chain")
			}
		})
	}
}

var errSentinel = errors.New("fetch failed")

// TestRedact_GeneratedInputs runs the generated inputs, built from every delimiter of the shared
// table, through every entry point.
func TestRedact_GeneratedInputs(t *testing.T) {
	for _, separator := range redacttest.Seeds() {
		secret := redacttest.SecretOf("A", separator, "B")
		for _, in := range redacttest.Generate(secret) {
			checkEntryPoints(t, in)
		}
	}
}

// checkEntryPoints runs one generated input through every entry point of this package.
func checkEntryPoints(t testing.TB, in redacttest.Input) {
	t.Helper()
	redacttest.RequireNoLeak(t, "RedactURL", in, RedactURL(in.Raw))
	for _, message := range in.Messages() {
		redacttest.RequireNoLeak(t, "RedactCredentials", in, RedactCredentials(message, in.Raw))
		redacttest.RequireNoLeak(t, "RedactError", in, RedactError(fmt.Errorf("%w: %s", errSentinel, message), in.Raw).Error())
	}
}

// FuzzRedact embeds a random secret in many address templates and checks that no entry point
// leaks the user name or any fragment of the secret. The seeds are the delimiters of the shared
// table, so every input of the table is a starting point.
func FuzzRedact(f *testing.F) {
	for _, separator := range redacttest.Seeds() {
		f.Add("A", separator, "B")
		f.Add("", separator, "")
		f.Add("x"+separator+"y", separator, "z"+separator)
	}
	// Seeds past the bounds of the grammar: more "@" signs than a real address holds, and an address
	// longer than any real one. They must be withheld whole, never expanded.
	f.Add("A", strings.Repeat("@", maxBoundaries+8), "B")
	f.Add("A", strings.Repeat("@x", hostileBoundaries), "B")
	f.Add(strings.Repeat("a", maxAddressLength), "@", "B")
	f.Fuzz(func(t *testing.T, first, separator, second string) {
		secret := redacttest.SecretOf(first, separator, second)
		for _, in := range redacttest.Generate(secret) {
			checkEntryPoints(t, in)
		}
	})
}

func TestRedactCredentials(t *testing.T) {
	tests := []struct {
		name string
		text string
		raw  string
		want string
	}{
		{name: "no credential", text: "boom at example.com", raw: "https://example.com/a.yaml", want: "boom at example.com"},
		{name: "userinfo is cut out", text: "could not parse reference: user:hunter2@example.com/a", raw: "https://user:hunter2@example.com/a", want: "could not parse reference: example.com/a"},
		{name: "password quoted on its own", text: "auth failed for hunter2", raw: "https://user:hunter2@example.com/a", want: "auth failed for <redacted>"},
		{name: "encoded password is also removed decoded", text: "bad password p@ss", raw: "https://user:p%40ss@example.com/a", want: "bad password <redacted>"},
		{name: "double colon is part of the password", text: "bad password pa::ss for https://user:pa::ss@example.com/a", raw: "https://user:pa::ss@example.com/a", want: "bad password <redacted> for https://example.com/a"},
		{name: "last at sign ends the userinfo", text: "failed: https://user:pa@ss@second@example.com/a", raw: "https://user:pa@ss@second@example.com/a", want: "failed: https://example.com/a"},
		{name: "forced getter prefix", text: "clone https://user:hunter2@example.com/r.git failed", raw: "git::https://user:hunter2@example.com/r.git", want: "clone https://example.com/r.git failed"},
		{name: "user name alone is not a password", text: "git failed", raw: "ssh://git@example.com/r.git", want: "git failed"},
		{name: "digest at sign is not userinfo", text: "pull ghcr.io/a/b@" + sha256Digest + " failed", raw: "oci://ghcr.io/a/b@" + sha256Digest, want: "pull ghcr.io/a/b@" + sha256Digest + " failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RedactCredentials(tt.text, tt.raw))
		})
	}
}

var sha256Digest = "sha256:" + strings.Repeat("0f", 32)
