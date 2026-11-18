package authoritytest

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
)

// FakeToken is the credential a Fake accepts for every use.
const FakeToken = "fake-authority-token-0001"

// FakeCall is one call a Fake received.
type FakeCall struct {
	// Op is "ensure" or "revoke".
	Op          string
	Fingerprint string
	Uses        []authority.Use
}

// Fake is an in-memory authority for tests of code that uses authorities. It checks the token,
// models per-use grants, records its calls, and can be told to fail.
type Fake struct {
	name string
	uses []authority.Use

	mu          sync.Mutex
	grants      map[string]map[authority.Use]bool
	enrollments map[string]map[authority.Use]string
	calls       []FakeCall
	ensureErrs  []error
	revokeErrs  []error
	failOnUse   map[authority.Use]error
	seq         int
}

// NewFake returns a fake authority named name that supports uses, or both uses when none is given.
// FakeToken is accepted for every use.
func NewFake(name string, uses ...authority.Use) *Fake {
	if len(uses) == 0 {
		uses = []authority.Use{authority.UseAuthentication, authority.UseSigning}
	}
	f := &Fake{
		name:        name,
		uses:        authority.NormalizeUses(uses),
		grants:      map[string]map[authority.Use]bool{},
		enrollments: map[string]map[authority.Use]string{},
		failOnUse:   map[authority.Use]error{},
	}
	f.Grant(FakeToken, f.uses...)
	return f
}

// Grant lets token enroll and revoke the given uses. A token that was never granted is rejected.
func (f *Fake) Grant(token string, uses ...authority.Use) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := map[authority.Use]bool{}
	for _, use := range uses {
		g[use] = true
	}
	f.grants[token] = g
}

// FailNextEnsure makes the next Ensure call return err before it enrolls anything.
func (f *Fake) FailNextEnsure(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureErrs = append(f.ensureErrs, err)
}

// FailNextRevoke makes the next Revoke call return err without withdrawing anything.
func (f *Fake) FailNextRevoke(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeErrs = append(f.revokeErrs, err)
}

// FailEnsureOnUse makes Ensure fail when it reaches use, after it enrolled the earlier uses. A nil
// err clears the failure.
func (f *Fake) FailEnsureOnUse(use authority.Use, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.failOnUse, use)
		return
	}
	f.failOnUse[use] = err
}

// Calls returns the calls received so far.
func (f *Fake) Calls() []FakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeCall(nil), f.calls...)
}

// CallCount counts the calls of kind op ("ensure" or "revoke").
func (f *Fake) CallCount(op string) int {
	n := 0
	for _, c := range f.Calls() {
		if c.Op == op {
			n++
		}
	}
	return n
}

// Count returns how many enrollments the fake holds for key and use (0 or 1).
func (f *Fake) Count(key ssh.PublicKey, use authority.Use) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.enrollments[authority.Fingerprint(key)][use]; ok {
		return 1
	}
	return 0
}

// Enrolled lists the fingerprints that have at least one enrollment, in order.
func (f *Fake) Enrolled() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for fp, byUse := range f.enrollments {
		if len(byUse) > 0 {
			out = append(out, fp)
		}
	}
	sort.Strings(out)
	return out
}

// Name implements authority.Authority.
func (f *Fake) Name() string { return f.name }

// Supports implements authority.Authority.
func (f *Fake) Supports() []authority.Use { return append([]authority.Use(nil), f.uses...) }

func (f *Fake) check(operation, token string, use authority.Use) error {
	grant, known := f.grants[token]
	switch {
	case !known:
		return &authority.Error{Authority: f.name, Operation: operation, Err: authority.ErrUnauthorized, Detail: "unknown token"}
	case use != "" && !grant[use]:
		return &authority.Error{Authority: f.name, Operation: operation, Use: use, Err: authority.ErrMissingPermission, Permission: "enroll:" + string(use)}
	}
	return nil
}

// Ensure implements authority.Authority.
func (f *Fake) Ensure(_ context.Context, credentials map[string]string, request authority.Request) (authority.Receipt, error) {
	receipt := authority.NewReceipt(f.name, request)
	if err := authority.ValidateRequest(f, request); err != nil {
		return receipt, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	uses := authority.NormalizeUses(request.Uses)
	f.calls = append(f.calls, FakeCall{Op: "ensure", Fingerprint: receipt.Fingerprint, Uses: uses})
	if len(f.ensureErrs) > 0 {
		err := f.ensureErrs[0]
		f.ensureErrs = f.ensureErrs[1:]
		return receipt, err
	}
	token := credentials[authority.CredentialToken]
	if err := f.check("ensure", token, ""); err != nil {
		return receipt, err
	}
	receipt.Subject = "fake-user"
	byUse := f.enrollments[receipt.Fingerprint]
	if byUse == nil {
		byUse = map[authority.Use]string{}
		f.enrollments[receipt.Fingerprint] = byUse
	}
	for _, use := range uses {
		if err := f.check("ensure", token, use); err != nil {
			return receipt, err
		}
		if err := f.failOnUse[use]; err != nil {
			return receipt, err
		}
		id, ok := byUse[use]
		if !ok {
			f.seq++
			id = "fake-" + strconv.Itoa(f.seq)
			byUse[use] = id
		}
		receipt.References[use] = id
		receipt.Uses = append(receipt.Uses, use)
	}
	return receipt, nil
}

// Revoke implements authority.Authority.
func (f *Fake) Revoke(_ context.Context, credentials map[string]string, receipt authority.Receipt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, FakeCall{Op: "revoke", Fingerprint: receipt.Fingerprint, Uses: receipt.EnrolledUses()})
	if len(f.revokeErrs) > 0 {
		err := f.revokeErrs[0]
		f.revokeErrs = f.revokeErrs[1:]
		return err
	}
	if len(receipt.References) == 0 {
		return nil
	}
	token := credentials[authority.CredentialToken]
	if err := f.check("revoke", token, ""); err != nil {
		return err
	}
	for use, id := range receipt.References {
		if err := f.check("revoke", token, use); err != nil {
			return err
		}
		if f.enrollments[receipt.Fingerprint][use] == id {
			delete(f.enrollments[receipt.Fingerprint], use)
		}
	}
	return nil
}

// Environment wires the fake to the conformance suite.
func (f *Fake) Environment() *Environment {
	limited := func(use authority.Use) string { return "fake-limited-" + string(use) }
	for _, use := range f.uses {
		var others []authority.Use
		for _, o := range f.uses {
			if o != use {
				others = append(others, o)
			}
		}
		f.Grant(limited(use), others...)
	}
	return &Environment{
		Authority:   f,
		Credentials: map[string]string{authority.CredentialToken: FakeToken},
		Invalid:     map[string]string{authority.CredentialToken: "fake-unknown-token"},
		Restricted: func(use authority.Use) map[string]string {
			return map[string]string{authority.CredentialToken: limited(use)}
		},
		Permission: func(use authority.Use) string { return fmt.Sprintf("enroll:%s", use) },
		Seed: func(t *testing.T, key ssh.PublicKey, use authority.Use) string {
			t.Helper()
			f.mu.Lock()
			defer f.mu.Unlock()
			fp := authority.Fingerprint(key)
			if f.enrollments[fp] == nil {
				f.enrollments[fp] = map[authority.Use]string{}
			}
			f.seq++
			id := "fake-" + strconv.Itoa(f.seq)
			f.enrollments[fp][use] = id
			return id
		},
		Enrollments: f.Count,
		Secrets:     []string{FakeToken, "fake-unknown-token"},
	}
}
