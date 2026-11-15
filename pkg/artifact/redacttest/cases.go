// Package redacttest holds the credential-bearing inputs that every URL redaction helper must
// handle. The helpers in pkg/artifact and pkg/utils share one security property, so they share one
// table: a test feeds the same cases to each helper and asserts that no secret survives.
package redacttest

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// digestBytes is the number of repeated bytes in the sample digest of the OCI cases.
const digestBytes = 32

const (
	// User is a distinctive user name, so a test can tell it apart from any host or path.
	User = "usr7"
	// SecretA and SecretB are distinctive password fragments.
	SecretA = "S3cr3tA"
	// SecretB is the second password fragment, used when a delimiter splits a password in two.
	SecretB = "S3cr3tB"
)

// Case is one credential-bearing input.
type Case struct {
	// Name describes the shape of the input.
	Name string
	// Raw is the URL, or URL-like address, that carries the credential.
	Raw string
	// Secrets are the strings that must never appear in the redacted form of Raw: the user, the
	// password, and every fragment a delimiter inside the password could leave behind.
	Secrets []string
	// Quoted are the passwords an error may quote on their own, in any form a parser can decode.
	Quoted []string
}

// Cases returns the shared table of nasty inputs. Each call returns a fresh slice, so a caller may
// not alias or mutate another caller's table.
func Cases() []Case {
	defer perf.Track(nil, "redacttest.Cases")()

	digest := "sha256:" + strings.Repeat("ab", digestBytes)
	return []Case{
		{Name: "user and password", Raw: "https://usr7:S3cr3tA@example.com/a", Secrets: []string{User, SecretA}, Quoted: []string{SecretA}},
		{Name: "password with a colon", Raw: "https://usr7:S3cr3tA:S3cr3tB@example.com/a", Secrets: []string{User, SecretA, SecretB}, Quoted: []string{"S3cr3tA:S3cr3tB"}},
		{Name: "password with a double colon", Raw: "https://usr7:S3cr3tA::S3cr3tB@example.com/a", Secrets: []string{User, SecretA, SecretB}, Quoted: []string{"S3cr3tA::S3cr3tB"}},
		{Name: "password with an at sign", Raw: "https://usr7:S3cr3tA@S3cr3tB@example.com/a", Secrets: []string{User, SecretA, SecretB}, Quoted: []string{"S3cr3tA@S3cr3tB"}},
		{Name: "password with two at signs", Raw: "https://usr7:S3cr3tA@S3cr3tB@second@example.com/a", Secrets: []string{User, SecretA, SecretB, "second"}, Quoted: []string{"S3cr3tA@S3cr3tB@second"}},
		{Name: "password with a hash", Raw: "https://usr7:S3cr3tA#S3cr3tB@example.com/a", Secrets: []string{User, SecretA, SecretB}, Quoted: []string{"S3cr3tA#S3cr3tB"}},
		{Name: "password with a question mark", Raw: "https://usr7:S3cr3tA?S3cr3tB@example.com/a", Secrets: []string{User, SecretA, SecretB}, Quoted: []string{"S3cr3tA?S3cr3tB"}},
		{Name: "password with a slash", Raw: "https://usr7:S3cr3tA/S3cr3tB@example.com/a", Secrets: []string{User, SecretA, SecretB}, Quoted: []string{"S3cr3tA/S3cr3tB"}},
		{Name: "numeric password with a hash", Raw: "https://usr7:12345#S3cr3tB@example.com/a", Secrets: []string{User, "12345", SecretB}, Quoted: []string{"12345#S3cr3tB"}},
		{Name: "numeric password with a slash", Raw: "https://usr7:12345/S3cr3tB@example.com/a", Secrets: []string{User, "12345", SecretB}, Quoted: []string{"12345/S3cr3tB"}},
		{Name: "percent-escaped password", Raw: "https://usr7:S3cr3t%40Z@example.com/a", Secrets: []string{User, "S3cr3t%40Z", "S3cr3t@Z"}, Quoted: []string{"S3cr3t%40Z", "S3cr3t@Z"}},
		{Name: "malformed percent escape", Raw: "https://usr7:S3cr3t%zzZ@example.com/a", Secrets: []string{User, "S3cr3t%zzZ"}, Quoted: []string{"S3cr3t%zzZ"}},
		{Name: "empty user", Raw: "https://:S3cr3tA@example.com/a", Secrets: []string{SecretA}, Quoted: []string{SecretA}},
		{Name: "empty password", Raw: "https://usr7:@example.com/a", Secrets: []string{User}},
		{Name: "empty user and password", Raw: "https://:@example.com/a"},
		{Name: "token as user", Raw: "https://S3cr3tA@example.com/a", Secrets: []string{SecretA}},
		{Name: "ipv6 host", Raw: "https://usr7:S3cr3tA@[::1]:8443/a", Secrets: []string{User, SecretA}, Quoted: []string{SecretA}},
		{Name: "userinfo with a space", Raw: "https://usr7:S3cr3tA S3cr3tB@example.com/a", Secrets: []string{User, SecretA, SecretB}, Quoted: []string{"S3cr3tA S3cr3tB"}},
		{Name: "upper-case scheme", Raw: "HTTPS://usr7:S3cr3tA@example.com/a", Secrets: []string{User, SecretA}, Quoted: []string{SecretA}},
		{Name: "scheme-relative", Raw: "//usr7:S3cr3tA@example.com/a", Secrets: []string{User, SecretA}, Quoted: []string{SecretA}},
		{Name: "no scheme", Raw: "usr7:S3cr3tA@example.com/a", Secrets: []string{User, SecretA}, Quoted: []string{SecretA}},
		{Name: "forced getter", Raw: "git::https://usr7:S3cr3tA@example.com/r.git", Secrets: []string{User, SecretA}, Quoted: []string{SecretA}},
		{Name: "forced getter with subdirectory and query", Raw: "git::https://usr7:S3cr3tA@example.com/r.git//sub?ref=v1", Secrets: []string{User, SecretA}, Quoted: []string{SecretA}},
		{Name: "forced getter with a double colon password", Raw: "git::https://usr7:S3cr3tA::S3cr3tB@example.com/r.git", Secrets: []string{User, SecretA, SecretB}, Quoted: []string{"S3cr3tA::S3cr3tB"}},
		{Name: "oci reference with a digest", Raw: "oci://usr7:S3cr3tA@ghcr.io/acme/catalog@" + digest, Secrets: []string{User, SecretA}, Quoted: []string{SecretA}},
		{Name: "oci reference with a double colon password and a digest", Raw: "oci://usr7:S3cr3tA::S3cr3tB@ghcr.io/acme/catalog@" + digest, Secrets: []string{User, SecretA, SecretB}, Quoted: []string{"S3cr3tA::S3cr3tB"}},
	}
}
