// Package authority defines the parties that vouch for an SSH public key.
//
// An authority is asked, at login, to make sure the key is enrolled for the requested uses. GitHub
// is one authority: the key is registered on a user account as a signing key and/or an
// authentication key. The package also holds the registry of authorities, the on-disk receipt
// store, and Reconcile, which brings an authority and the receipts into agreement with the keys
// that exist on this machine.
package authority

import (
	"context"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// CredentialToken is the key under which an authority finds its token in the credentials map.
const CredentialToken = "token"

// Use is what an authority vouches for a key doing.
type Use string

const (
	// UseSigning lets the key sign, for example Git commits.
	UseSigning Use = "signing"
	// UseAuthentication lets the key authenticate, for example SSH logins and Git over SSH.
	UseAuthentication Use = "authentication"
)

// Authority vouches for a public key using credentials supplied by the caller.
type Authority interface {
	// Name is the registry key, such as "github".
	Name() string
	// Supports lists the uses it can vouch for.
	Supports() []Use
	// Ensure makes the authority trust the key for the requested uses. It is idempotent.
	//
	// When Ensure fails after enrolling some uses, it returns the receipt of what was enrolled
	// along with the error, so the caller can persist the references needed to revoke later.
	Ensure(ctx context.Context, credentials map[string]string, request Request) (Receipt, error)
	// Revoke withdraws an enrollment. It is idempotent: a missing enrollment is success. Only the
	// uses named in receipt.References are withdrawn.
	Revoke(ctx context.Context, credentials map[string]string, receipt Receipt) error
}

// Request asks an authority to vouch for a key.
type Request struct {
	// KeyName is the local name of the key, recorded in the receipt.
	KeyName string
	// Key is the public key to enroll.
	Key ssh.PublicKey
	// Uses is what the key is to be trusted for.
	Uses []Use
	// Title labels the key on the authority, such as "<key>@<hostname> (Atmos)".
	Title string
	// Principals are remote usernames, for authorities that issue certificates.
	Principals []string
	// Target is a host or instance, for authorities that enroll per host.
	Target string
}

// Receipt records that an authority trusts a key. It holds no credential.
type Receipt struct {
	// Authority is the name of the authority that issued the receipt.
	Authority string `json:"authority"`
	// Subject is the account that now trusts the key.
	Subject string `json:"subject,omitempty"`
	// KeyName is the local name of the key.
	KeyName string `json:"key_name"`
	// Fingerprint is the SHA256 fingerprint of the public key.
	Fingerprint string `json:"fingerprint"`
	// PublicKey is the public key in authorized_keys form.
	PublicKey string `json:"public_key,omitempty"`
	// Uses lists what the authority was asked to vouch for.
	Uses []Use `json:"uses,omitempty"`
	// References maps a use to the authority's own ID, which revocation needs.
	References map[Use]string `json:"references,omitempty"`
	// Certificate is set by authorities that issue certificates.
	Certificate []byte `json:"certificate,omitempty"`
	// Created is when the receipt was first issued.
	Created time.Time `json:"created"`
	// ExpiresAt is when the trust ends. The zero value means trusted until revoked.
	ExpiresAt time.Time `json:"expires_at,omitzero"`
}

// Fingerprint returns the SHA256 fingerprint of key.
func Fingerprint(key ssh.PublicKey) string {
	return ssh.FingerprintSHA256(key)
}

// AuthorizedKey returns key in authorized_keys form, without a trailing newline.
func AuthorizedKey(key ssh.PublicKey) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

// NewReceipt returns a receipt for request with the fields every authority fills in the same way.
func NewReceipt(authorityName string, request Request) Receipt {
	return Receipt{
		Authority:   authorityName,
		KeyName:     request.KeyName,
		Fingerprint: Fingerprint(request.Key),
		PublicKey:   AuthorizedKey(request.Key),
		References:  map[Use]string{},
		Created:     time.Now().UTC(),
	}
}

// NormalizeUses returns uses without duplicates or empty values, in a stable order.
func NormalizeUses(uses []Use) []Use {
	seen := make(map[Use]bool, len(uses))
	out := make([]Use, 0, len(uses))
	for _, use := range uses {
		if use == "" || seen[use] {
			continue
		}
		seen[use] = true
		out = append(out, use)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// EnrolledUses returns every use the receipt covers or holds a reference for.
func (r Receipt) EnrolledUses() []Use {
	all := make([]Use, 0, len(r.Uses)+len(r.References))
	all = append(all, r.Uses...)
	for use := range r.References {
		all = append(all, use)
	}
	return NormalizeUses(all)
}

// Expired reports whether the receipt's trust has ended at now.
func (r Receipt) Expired(now time.Time) bool {
	return !r.ExpiresAt.IsZero() && !now.Before(r.ExpiresAt)
}

// ValidateRequest checks request before any network call: a key, at least one use, and only uses
// the authority supports.
func ValidateRequest(a Authority, request Request) error {
	if request.Key == nil {
		return &Error{Authority: a.Name(), Operation: "ensure", Err: ErrInvalidRequest, Detail: "no public key"}
	}
	uses := NormalizeUses(request.Uses)
	if len(uses) == 0 {
		return &Error{Authority: a.Name(), Operation: "ensure", Err: ErrInvalidRequest, Detail: "no uses requested"}
	}
	supported := make(map[Use]bool)
	for _, use := range a.Supports() {
		supported[use] = true
	}
	for _, use := range uses {
		if !supported[use] {
			return &Error{Authority: a.Name(), Operation: "ensure", Use: use, Err: ErrUnsupportedUse}
		}
	}
	return nil
}

// equalUses reports whether a and b hold the same uses, ignoring order and duplicates.
func equalUses(a, b []Use) bool {
	a, b = NormalizeUses(a), NormalizeUses(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// differenceUses returns the uses in a that are not in b.
func differenceUses(a, b []Use) []Use {
	skip := make(map[Use]bool, len(b))
	for _, use := range b {
		skip[use] = true
	}
	var out []Use
	for _, use := range NormalizeUses(a) {
		if !skip[use] {
			out = append(out, use)
		}
	}
	return out
}
